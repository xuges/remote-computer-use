package backend

import (
	ffi "github.com/goexlib/cgo"
	"golang.org/x/sys/windows"
)

func loadLibrary(path string) (ffi.Library, error) {
	h, e := windows.LoadLibraryEx(path, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	return ffi.Library(h), e
}
