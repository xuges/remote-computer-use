//go:build !windows

package host

import (
	"errors"
	"os"
	"syscall"
)

func mkdirPrivate(path string) error { return os.MkdirAll(path, 0700) }

func Protect(path string, directory bool) error {
	mode := os.FileMode(0600)
	if directory {
		mode = 0700
	}
	return os.Chmod(path, mode)
}
func CheckPrivate(path string, directory bool) error {
	s, e := os.Lstat(path)
	if e != nil {
		return e
	}
	stat, ok := s.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || s.Mode().Perm()&0077 != 0 || s.IsDir() != directory || (!directory && !s.Mode().IsRegular()) {
		return errors.New("unsafe file permissions")
	}
	return nil
}
