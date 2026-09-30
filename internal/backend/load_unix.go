//go:build !windows

package backend

import ffi "github.com/goexlib/cgo"

func loadLibrary(path string) (ffi.Library, error) { return ffi.LoadLibrary(path) }
