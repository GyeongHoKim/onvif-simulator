package ffmpeg

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/obs"
)

// stderr scanner sizing. The bufio.Scanner default is 64 KiB; a 256 KiB
// ceiling covers verbose IPA / banner output without unbounded memory.
const (
	initStderrLineBytes = 64 * 1024
	maxStderrLineBytes  = 256 * 1024
)

// videoClockRate is the RTP clock rate (90 kHz) used to stamp MJPEG
// frames. Matches the H.264/H.265 paths so an operator who pairs an
// MJPEG sibling with its H.264 source sees consistent timestamping.
const videoClockRate = 90000

// defaultJPEGQuality matches mediamtx's stock value: ffmpeg's -q:v
// scale runs from 2 (best) to 31 (worst) and 5 produces a clean
// reference image without bloating frame size.
const defaultJPEGQuality = 5

// buildFFmpegArgs renders Params into the exact ffmpeg flag list the
// transcoder spawns. Extracted as a pure function so the wiring can
// be unit tested without spawning a subprocess.
func buildFFmpegArgs(p Params) []string {
	quality := p.Quality
	if quality <= 0 {
		quality = defaultJPEGQuality
	}
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-fflags", "+genpts",
		"-probesize", "32",
		"-analyzeduration", "0",
	}
	if p.LoopForever {
		args = append(args, "-stream_loop", "-1")
	}
	args = append(args,
		"-re", // pace input at real-time so the simulator's MJPEG cadence matches the source
		"-i", p.MediaFilePath,
		"-c:v", "mjpeg",
		"-q:v", strconv.Itoa(quality),
		"-pix_fmt", "yuvj420p", // MJPEG encoder requires JPEG color range
	)
	if p.FPS > 0 {
		args = append(args, "-r", strconv.Itoa(p.FPS))
	}
	args = append(args, "-f", "mpjpeg", "pipe:1")
	return args
}

// stderrTailLines bounds the post-mortem ffmpeg stderr snapshot. ffmpeg
// is noisy at -v info — 64 lines holds enough banner + error context
// without unbounded memory if the helper goes haywire.
const stderrTailLines = 64

// errTerminated is the sentinel run() returns on a clean Close(). Lets
// the caller suppress the post-mortem WARN on normal shutdown.
var errTerminated = errors.New("ffmpeg: terminated")

// errOnJPEGRequired is the sentinel Open returns when the caller passes
// a nil onJPEG callback — non-nil is required because the Transcoder
// has nowhere to deliver frames without it.
var errOnJPEGRequired = errors.New("ffmpeg: onJPEG callback must be non-nil")

// errMediaFilePathRequired is the sentinel Open returns when Params
// arrives without a source file path.
var errMediaFilePathRequired = errors.New("ffmpeg: Params.MediaFilePath must be set")

// extractBinaryFn / argFactory are package-level seams so tests can
// swap in a fake ffmpeg subprocess (typically the test binary itself in
// helper-process mode) without going through the real //go:embed path.
// Production code never sets them — defaults route to the real
// implementations in this file/binary.go.
var (
	extractBinaryFn = extractBinary
	argFactory      = buildFFmpegArgs
)

// Transcoder is a running ffmpeg subprocess feeding JPEG frames to the
// configured callback. It is constructed by Open and torn down by
// Close; Wait blocks until the process exits.
type Transcoder struct {
	logger *slog.Logger
	onJPEG OnJPEGDataFunc
	fps    int

	cleanup func()
	cmd     *exec.Cmd
	stdout  io.ReadCloser
	stderr  io.ReadCloser

	stderrDone chan struct{}
	tail       *stderrTail

	terminate chan struct{}
	done      chan struct{}
	finalErr  error

	closeOnce sync.Once
}

// Open extracts the embedded ffmpeg binary, fork+execs it with the
// configured input, and starts a reader goroutine that parses the
// mpjpeg stream and invokes onJPEG for each frame. logger may be nil.
// onJPEG must be non-nil.
func Open(p Params, logger *slog.Logger, onJPEG OnJPEGDataFunc) (*Transcoder, error) {
	if onJPEG == nil {
		return nil, errOnJPEGRequired
	}
	if p.MediaFilePath == "" {
		return nil, errMediaFilePathRequired
	}
	if logger == nil {
		logger = obs.Discard()
	}

	binPath, cleanup, err := extractBinaryFn()
	if err != nil {
		return nil, err
	}

	args := argFactory(p)

	// CommandContext lets a future shutdown path tear the child down
	// promptly; today we drive shutdown via cmd.Process.Kill in run().
	//nolint:gosec // binPath is our own extracted binary
	cmd := exec.CommandContext(context.Background(), binPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("ffmpeg: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("ffmpeg: stderr pipe: %w", err)
	}

	t := &Transcoder{
		logger:     logger,
		onJPEG:     onJPEG,
		fps:        p.FPS,
		cleanup:    cleanup,
		cmd:        cmd,
		stdout:     stdout,
		stderr:     stderr,
		stderrDone: make(chan struct{}),
		tail:       newStderrTail(stderrTailLines),
		terminate:  make(chan struct{}),
		done:       make(chan struct{}),
	}

	if startErr := cmd.Start(); startErr != nil {
		_ = stdout.Close() //nolint:errcheck // cleanup; original error is startErr
		_ = stderr.Close() //nolint:errcheck // cleanup; original error is startErr
		cleanup()
		return nil, fmt.Errorf("ffmpeg: start subprocess: %w", startErr)
	}

	go t.drainStderr()
	go t.run()

	return t, nil
}

// Close signals the transcoder to stop and waits for it. Idempotent.
func (t *Transcoder) Close() {
	t.closeOnce.Do(func() {
		close(t.terminate)
		<-t.done
		t.cleanup()
	})
}

// Wait blocks until the transcoder exits and returns its terminal error
// (nil on clean shutdown, including a Close-initiated stop).
func (t *Transcoder) Wait() error {
	<-t.done
	if errors.Is(t.finalErr, errTerminated) {
		return nil
	}
	return t.finalErr
}

func (t *Transcoder) run() {
	defer close(t.done)
	t.finalErr = t.runInner()
	if t.finalErr != nil && !errors.Is(t.finalErr, errTerminated) {
		t.flushStderrTail(t.finalErr)
	}
}

func (t *Transcoder) runInner() error {
	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- t.cmd.Wait()
	}()

	readDone := make(chan error, 1)
	go func() {
		readDone <- t.runReader()
	}()

	for {
		select {
		case err := <-cmdDone:
			<-readDone
			_ = t.stdout.Close() //nolint:errcheck // shutdown best-effort
			<-t.stderrDone
			return err
		case err := <-readDone:
			_ = t.cmd.Process.Kill() //nolint:errcheck // best-effort terminate on reader exit
			<-cmdDone
			<-t.stderrDone
			return err
		case <-t.terminate:
			_ = t.cmd.Process.Kill() //nolint:errcheck // best-effort terminate on Close
			<-cmdDone
			<-readDone
			_ = t.stdout.Close() //nolint:errcheck // shutdown best-effort
			<-t.stderrDone
			return errTerminated
		}
	}
}

// runReader pulls JPEG frames from the mpjpeg stream and dispatches
// them to onJPEG. PTS is synthesized from a monotonic frame counter
// against the configured FPS (or 30 if unset) so the RTP timeline is
// strictly increasing even if ffmpeg's internal pacing jitters.
func (t *Transcoder) runReader() error {
	br := bufio.NewReader(t.stdout)
	fps := t.fps
	if fps <= 0 {
		fps = 30
	}
	tickIncrement := int64(videoClockRate / fps)
	if tickIncrement == 0 {
		tickIncrement = videoClockRate / 30
	}

	var pts int64
	ctx := context.Background()
	for {
		frame, err := readMPJPEGFrame(ctx, br)
		if err != nil {
			if errors.Is(err, errReaderClosed) {
				return nil
			}
			return err
		}
		t.onJPEG(pts, time.Now(), frame)
		pts += tickIncrement
	}
}

// drainStderr mirrors helper stderr into the simulator log at DEBUG and
// captures the tail for post-mortem WARN flushing on failure.
func (t *Transcoder) drainStderr() {
	defer close(t.stderrDone)
	scanner := bufio.NewScanner(t.stderr)
	scanner.Buffer(make([]byte, initStderrLineBytes), maxStderrLineBytes)
	for scanner.Scan() {
		line := scanner.Text()
		t.tail.push(line)
		t.logger.Debug("ffmpeg: stderr", "line", line)
	}
	if err := scanner.Err(); err != nil {
		t.tail.push(fmt.Sprintf("<stderr scanner error: %s>", err))
		t.logger.Warn("ffmpeg: stderr scanner stopped", "err", err)
	}
}

func (t *Transcoder) flushStderrTail(cause error) {
	lines := t.tail.snapshot()
	if len(lines) == 0 {
		return
	}
	t.logger.Warn("ffmpeg: stderr tail",
		"err", cause.Error(),
		"lines", lines,
	)
}
