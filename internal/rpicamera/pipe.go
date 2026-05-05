//go:build rpicam && linux && (arm || arm64)

package rpicamera

import "syscall"

// pipe is a length-prefixed message pipe over a syscall.Pipe(2) pair. The
// 4-byte little-endian header lets the reader recover frame boundaries
// without parsing the payload — the wire format matches mtxrpicam.
type pipe struct {
	readFD  int
	writeFD int
}

func newPipe() (*pipe, error) {
	fds := make([]int, 2)
	if err := syscall.Pipe(fds); err != nil {
		return nil, err
	}
	return &pipe{readFD: fds[0], writeFD: fds[1]}, nil
}

func (p *pipe) close() {
	_ = syscall.Close(p.readFD)
	_ = syscall.Close(p.writeFD)
}

func (p *pipe) read() ([]byte, error) {
	hdr := make([]byte, 4)
	if err := syscallReadAll(p.readFD, hdr); err != nil {
		return nil, err
	}
	le := int(hdr[3])<<24 | int(hdr[2])<<16 | int(hdr[1])<<8 | int(hdr[0])
	buf := make([]byte, le)
	if err := syscallReadAll(p.readFD, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (p *pipe) write(byts []byte) error {
	le := len(byts)
	hdr := []byte{byte(le), byte(le >> 8), byte(le >> 16), byte(le >> 24)}
	if _, err := syscall.Write(p.writeFD, hdr); err != nil {
		return err
	}
	_, err := syscall.Write(p.writeFD, byts)
	return err
}

func syscallReadAll(fd int, buf []byte) error {
	size := len(buf)
	read := 0
	for {
		n, err := syscall.Read(fd, buf[read:size])
		if err != nil {
			return err
		}
		read += n
		if read >= size {
			break
		}
	}
	return nil
}
