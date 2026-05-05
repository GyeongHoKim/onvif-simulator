//go:build rpicam && linux && (arm || arm64)

package rpicamera

import (
	"debug/elf"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"

	"github.com/GyeongHoKim/onvif-simulator/internal/obs"
)

const (
	libraryToCheckArchitecture = "libc.so.6"
	dumpPrefix                 = "/dev/shm/onvif-simulator-rpicamera-"
	executableName             = "mtxrpicam"
)

var (
	dumpMutex sync.Mutex
	dumpCount = 0
	dumpPath  = ""
)

// OnDataFunc receives one H.264 access unit per call. pts is in the 90 kHz
// RTP clock; ntp is wall-clock time anchored to when the frame was captured.
type OnDataFunc func(pts int64, ntp time.Time, au [][]byte)

// Available reports whether the embedded mtxrpicam helper is wired into
// this binary and the OS architecture matches. Returns nil on a Pi-ready
// build; ErrUnsupported otherwise. Use this to detect support without
// actually spawning the helper.
func Available() error { return nil }

// Camera is an active mtxrpicam helper process. It is created by Open and
// shut down by Close; Wait blocks until the helper exits.
type Camera struct {
	logger *slog.Logger
	onData OnDataFunc

	cmd      *exec.Cmd
	pipeOut  *pipe
	pipeIn   *pipe
	finalErr error

	terminate chan struct{}
	done      chan struct{}
}

// Open extracts and starts the embedded mtxrpicam helper, returning a
// running Camera. logger may be nil. onData must be non-nil.
func Open(p Params, logger *slog.Logger, onData OnDataFunc) (*Camera, error) {
	if onData == nil {
		return nil, fmt.Errorf("rpicamera: onData must be non-nil")
	}
	if logger == nil {
		logger = obs.Discard()
	}

	c := &Camera{logger: logger, onData: onData}

	if err := dumpComponent(); err != nil {
		return nil, err
	}

	var err error
	c.pipeOut, err = newPipe()
	if err != nil {
		freeComponent()
		return nil, err
	}

	c.pipeIn, err = newPipe()
	if err != nil {
		c.pipeOut.close()
		freeComponent()
		return nil, err
	}

	env := []string{
		"PIPE_CONF_FD=" + strconv.FormatInt(int64(c.pipeOut.readFD), 10),
		"PIPE_VIDEO_FD=" + strconv.FormatInt(int64(c.pipeIn.writeFD), 10),
		"LD_LIBRARY_PATH=" + dumpPath,
	}

	c.cmd = exec.Command(filepath.Join(dumpPath, executableName)) //nolint:gosec
	// Discard helper output so it does not leak into the simulator's stdout
	// (reserved for user-facing CLI output) or stderr. Helper-side errors
	// reach us via the 'e' control byte on the read pipe.
	c.cmd.Stdout = io.Discard
	c.cmd.Stderr = io.Discard
	c.cmd.Env = env
	c.cmd.Dir = dumpPath
	// Detach the subprocess from the parent's process group so SIGINT/SIGTERM
	// to the simulator do not also kill mtxrpicam mid-frame; we shut it down
	// explicitly via the control pipe.
	c.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := c.cmd.Start(); err != nil {
		c.pipeOut.close()
		c.pipeIn.close()
		freeComponent()
		return nil, err
	}

	c.terminate = make(chan struct{})
	c.done = make(chan struct{})

	go c.run()

	if err := c.pipeOut.write(append([]byte{'c'}, p.hydrate().serialize()...)); err != nil {
		c.Close()
		return nil, err
	}

	return c, nil
}

// Close signals the helper to exit and waits for it.
func (c *Camera) Close() {
	close(c.terminate)
	<-c.done
	freeComponent()
}

// Wait blocks until the camera exits and returns its terminal error.
func (c *Camera) Wait() error {
	<-c.done
	return c.finalErr
}

// ReloadParams re-sends the capture parameters to a running helper. Today
// the simulator does not call this (live param edits are stop/edit/start);
// it is exposed for parity with mediamtx and for the deferred hot-reload
// follow-up.
func (c *Camera) ReloadParams(p Params) error {
	return c.pipeOut.write(append([]byte{'c'}, p.hydrate().serialize()...))
}

func (c *Camera) run() {
	defer close(c.done)
	c.finalErr = c.runInner()
}

func (c *Camera) runInner() error {
	cmdDone := make(chan error)
	go func() {
		cmdDone <- c.cmd.Wait()
	}()

	readDone := make(chan error)
	go func() {
		readDone <- c.runReader()
	}()

	for {
		select {
		case err := <-cmdDone:
			c.pipeIn.close()
			c.pipeOut.close()
			<-readDone
			return err

		case err := <-readDone:
			_ = c.pipeOut.write([]byte{'e'})
			<-cmdDone
			c.pipeIn.close()
			c.pipeOut.close()
			return err

		case <-c.terminate:
			_ = c.pipeOut.write([]byte{'e'})
			<-cmdDone
			c.pipeOut.close()
			c.pipeIn.close()
			<-readDone
			return fmt.Errorf("rpicamera: terminated")
		}
	}
}

func (c *Camera) runReader() error {
	// Wait for the helper's "ready" sentinel before forwarding access units.
	for {
		buf, err := c.pipeIn.read()
		if err != nil {
			return err
		}
		if len(buf) == 0 {
			return fmt.Errorf("rpicamera: empty control frame during init")
		}
		switch buf[0] {
		case 'e':
			return fmt.Errorf("rpicamera: helper error: %s", helperErrorMessage(buf))
		case 'r':
			goto streaming
		default:
			return fmt.Errorf("rpicamera: unexpected control byte 0x%.2x during init", buf[0])
		}
	}

streaming:
	for {
		buf, err := c.pipeIn.read()
		if err != nil {
			return err
		}
		if len(buf) == 0 {
			return fmt.Errorf("rpicamera: empty control frame")
		}
		switch buf[0] {
		case 'e':
			return fmt.Errorf("rpicamera: helper error: %s", helperErrorMessage(buf))

		case 'd':
			if len(buf) < 9 {
				return fmt.Errorf("rpicamera: short data frame (%d bytes)", len(buf))
			}
			dts := int64(buf[8])<<56 | int64(buf[7])<<48 | int64(buf[6])<<40 | int64(buf[5])<<32 |
				int64(buf[4])<<24 | int64(buf[3])<<16 | int64(buf[2])<<8 | int64(buf[1])

			var nalus h264.AnnexB
			if err := nalus.Unmarshal(buf[9:]); err != nil {
				return err
			}

			unixNTP := ntpTime()
			unixMono := monotonicTime()
			// Sec/Nsec are int32 on linux/arm and int64 on linux/arm64;
			// convert through int64 so both architectures compile cleanly.
			ntp := time.Unix(int64(unixNTP.Sec), int64(unixNTP.Nsec))
			deltaT := time.Duration(unixMono.Nano()-dts*1e3) * time.Nanosecond
			ntp = ntp.Add(-deltaT)

			c.onData(multiplyAndDivide(dts, 90000, 1e6), ntp, nalus)

		case 's':
			// Secondary MJPEG stream — currently unused; drop silently so a
			// future enabling of the secondary track does not break compat.

		default:
			return fmt.Errorf("rpicamera: unexpected control byte 0x%.2x", buf[0])
		}
	}
}

// helperErrorMessage extracts the message payload that follows an 'e' control
// byte, returning a placeholder when the helper sent only the byte itself.
func helperErrorMessage(buf []byte) string {
	if len(buf) < 2 {
		return "<empty>"
	}
	return string(buf[1:])
}

func ntpTime() syscall.Timespec {
	var t syscall.Timespec
	_, _, _ = syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 0, uintptr(unsafe.Pointer(&t)), 0)
	return t
}

func monotonicTime() syscall.Timespec {
	var t syscall.Timespec
	_, _, _ = syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 1, uintptr(unsafe.Pointer(&t)), 0)
	return t
}

// multiplyAndDivide computes v*m/d without overflowing int64 when v is
// already on the order of microseconds since the epoch.
func multiplyAndDivide(v, m, d int64) int64 {
	secs := v / d
	dec := v % d
	return secs*m + dec*m/d
}

func getArchitecture(libPath string) (bool, error) {
	f, err := os.Open(libPath) //nolint:gosec
	if err != nil {
		return false, err
	}
	defer f.Close()

	ef, err := elf.NewFile(f)
	if err != nil {
		return false, err
	}
	defer ef.Close()

	return ef.FileHeader.Class == elf.ELFCLASS64, nil
}

// checkArchitecture verifies that the running OS matches the embedded
// binary's word size. Mismatches are fatal because mtxrpicam_32 cannot run
// on a 64-bit userland and vice versa.
func checkArchitecture() error {
	byts, err := exec.Command("/sbin/ldconfig", "-p").Output()
	if err != nil {
		return fmt.Errorf("rpicamera: ldconfig failed: %w", err)
	}

	for _, line := range strings.Split(string(byts), "\n") {
		f := strings.Split(line, " => ")
		if len(f) == 2 && strings.Contains(f[1], libraryToCheckArchitecture) {
			is64, err := getArchitecture(f[1])
			if err != nil {
				return err
			}
			if runtime.GOARCH == "arm" {
				if !is64 {
					return nil
				}
			} else {
				if is64 {
					return nil
				}
			}
		}
	}

	if runtime.GOARCH == "arm" {
		return fmt.Errorf("rpicamera: the OS is 64-bit; install the arm64 build")
	}
	return fmt.Errorf("rpicamera: the OS is 32-bit; install the arm build")
}

func dumpEmbedFSRecursive(src, dest string) error {
	files, err := mtxrpicamFS.ReadDir(src)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() {
			if err := os.Mkdir(filepath.Join(dest, f.Name()), 0o755); err != nil {
				return err
			}
			if err := dumpEmbedFSRecursive(filepath.Join(src, f.Name()), filepath.Join(dest, f.Name())); err != nil {
				return err
			}
			continue
		}
		buf, err := mtxrpicamFS.ReadFile(filepath.Join(src, f.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dest, f.Name()), buf, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func dumpComponent() error {
	dumpMutex.Lock()
	defer dumpMutex.Unlock()

	if dumpCount > 0 {
		dumpCount++
		return nil
	}

	if err := checkArchitecture(); err != nil {
		return err
	}

	dumpPath = dumpPrefix + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.Mkdir(dumpPath, 0o755); err != nil {
		return err
	}

	files, err := mtxrpicamFS.ReadDir(".")
	if err != nil {
		_ = os.RemoveAll(dumpPath)
		return err
	}
	if len(files) == 0 {
		_ = os.RemoveAll(dumpPath)
		return fmt.Errorf("rpicamera: embedded mtxrpicam FS is empty")
	}

	if err := dumpEmbedFSRecursive(files[0].Name(), dumpPath); err != nil {
		_ = os.RemoveAll(dumpPath)
		return err
	}

	if err := os.Chmod(filepath.Join(dumpPath, executableName), 0o700); err != nil {
		_ = os.RemoveAll(dumpPath)
		return err
	}

	dumpCount++
	return nil
}

func freeComponent() {
	dumpMutex.Lock()
	defer dumpMutex.Unlock()
	dumpCount--
	if dumpCount == 0 {
		_ = os.RemoveAll(dumpPath)
	}
}
