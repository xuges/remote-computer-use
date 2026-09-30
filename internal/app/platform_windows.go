package app

import (
	"github.com/xuges/remote-computer-use/internal/core"
	"github.com/xuges/remote-computer-use/internal/host"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func runtimeDir() (string, error) {
	p := os.Getenv("REMOTE_COMPUTER_RUN_DIR")
	if p == "" {
		root, e := os.UserCacheDir()
		if e != nil {
			return "", core.IOError
		}
		p = filepath.Join(root, "remote-computer-use", "run")
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
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return nil, core.IOError
	}
	h, e := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e == windows.ERROR_SHARING_VIOLATION {
		return nil, core.NotReady
	}
	if e != nil {
		return nil, core.IOError
	}
	f := os.NewFile(uintptr(h), path)
	var info windows.ByHandleFileInformation
	if e = windows.GetFileInformationByHandle(h, &info); e != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		f.Close()
		return nil, core.IOError
	}
	return f, nil
}
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP}
}
func startupPipe() (*os.File, error) {
	var dup windows.Handle
	proc := windows.CurrentProcess()
	if e := windows.DuplicateHandle(proc, windows.Handle(os.Stdout.Fd()), proc, &dup, 0, false, windows.DUPLICATE_SAME_ACCESS); e != nil {
		return nil, e
	}
	ready := os.NewFile(uintptr(dup), "startup")
	null, e := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if e != nil {
		ready.Close()
		return nil, e
	}
	// Keep NUL handles alive until process exit; subsequently loaded C runtimes
	// initialize their standard streams from these Win32 handles.
	if e = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(null.Fd())); e == nil {
		e = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(null.Fd()))
	}
	if e != nil {
		ready.Close()
		null.Close()
		return nil, e
	}
	os.Stdout = null
	os.Stderr = null
	return ready, nil
}
