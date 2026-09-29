package app

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/xuges/remote-computer-use/internal/core"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func runtimeDir() (string, error) {
	p := os.Getenv("REMOTE_COMPUTER_RUN_DIR")
	if p == "" {
		if root := os.Getenv("XDG_RUNTIME_DIR"); root != "" {
			p = filepath.Join(root, "remote-computer-use")
		} else {
			p = filepath.Join(os.TempDir(), fmt.Sprintf("remote-computer-use-%d", os.Getuid()))
		}
	}
	p, err := filepath.Abs(p)
	if err != nil || len(p) > 58 {
		return "", core.IOError
	}
	if err = os.MkdirAll(p, 0700); err != nil {
		return "", core.IOError
	}
	s, err := os.Lstat(p)
	if err != nil || !s.IsDir() || s.Mode().Perm()&0077 != 0 {
		return "", core.IOError
	}
	if stat, ok := s.Sys().(*syscall.Stat_t); !ok || stat.Uid != uint32(os.Getuid()) {
		return "", core.IOError
	}
	return p, nil
}

func socketPath(dir, name string) string { return filepath.Join(dir, name+".sock") }

const maxPacket = 128 << 20

func writePacket(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil || len(b) > maxPacket {
		return core.IOError
	}
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], uint32(len(b)))
	if err = writeAll(w, h[:]); err != nil {
		return err
	}
	return writeAll(w, b)
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, e := w.Write(b)
		if e != nil {
			return e
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
func readPacket(r io.Reader, v any) error {
	var h [4]byte
	if _, e := io.ReadFull(r, h[:]); e != nil {
		return e
	}
	n := binary.BigEndian.Uint32(h[:])
	if n == 0 || n > maxPacket {
		return core.IOError
	}
	b := make([]byte, n)
	if _, e := io.ReadFull(r, b); e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}

func exchange(dir, name string, req core.Request) core.Result {
	c, e := net.DialTimeout("unix", socketPath(dir, name), time.Second)
	if e != nil {
		return core.Result{Error: core.NotConnected}
	}
	defer c.Close()
	d := req.Timeout
	if d <= 0 {
		d = 10 * time.Second
	}
	c.SetDeadline(time.Now().Add(d + 2*time.Second))
	if writePacket(c, req) != nil {
		return core.Result{Error: core.NetworkError}
	}
	var res core.Result
	if readPacket(c, &res) != nil {
		return core.Result{Error: core.NetworkError}
	}
	return res
}
