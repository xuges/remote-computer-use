//go:build !windows

package app

import (
	"fmt"
	"github.com/xuges/remote-computer-use/internal/core"
	"github.com/xuges/remote-computer-use/internal/host"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func runtimeDir() (string, error) {
	p := os.Getenv("REMOTE_COMPUTER_RUN_DIR")
	if p == "" {
		root := os.Getenv("XDG_RUNTIME_DIR")
		if root != "" {
			p = filepath.Join(root, "remote-computer-use")
		} else {
			p = fmt.Sprintf("/tmp/remote-computer-use-%d", os.Getuid())
		}
	}
	p, e := filepath.Abs(p)
	if e != nil {
		return "", core.IOError
	}
	if e = host.MkdirPrivate(p); e != nil {
		return "", core.IOError
	}
	return p, nil
}
func lockFile(path string) (*os.File, error) {
	fd, e := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return nil, core.IOError
	}
	f := os.NewFile(uintptr(fd), path)
	if e = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, core.NotReady
	}
	return f, nil
}
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
func startupPipe() (*os.File, error) {
	fd, e := unix.Dup(1)
	if e != nil {
		return nil, e
	}
	unix.CloseOnExec(fd)
	ready := os.NewFile(uintptr(fd), "startup")
	null, e := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if e != nil {
		ready.Close()
		return nil, e
	}
	defer null.Close()
	if e = unix.Dup2(int(null.Fd()), 1); e == nil {
		e = unix.Dup2(int(null.Fd()), 2)
	}
	if e != nil {
		ready.Close()
		return nil, e
	}
	return ready, nil
}
