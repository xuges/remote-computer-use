package backend

import (
	"context"
	ffi "github.com/goexlib/cgo"
	"github.com/xuges/remote-computer-use/internal/config"
	"github.com/xuges/remote-computer-use/internal/core"
	"github.com/xuges/remote-computer-use/internal/runtimebundle"
	"path/filepath"
	"runtime"
	"sync"
	"time"
	"unsafe"
)

type native struct {
	mu       sync.RWMutex
	lib      ffi.Library
	handle   uintptr
	status   func(uintptr, *[5]uint64) int32
	snapshot func(uintptr, *[5]uint64) unsafe.Pointer
	free     func(unsafe.Pointer)
	submit   func(uintptr, int32, int32, int32, int32, int32) uint64
	result   func(uintptr, uint64) int32
	cancel   func(uintptr, uint64)
	close    func(uintptr)
}

func Open(c config.Private) (Backend, error) {
	if !supportedPlatform() {
		return nil, core.Unsupported
	}
	dir, e := runtimebundle.Directory()
	if e != nil {
		return nil, core.MissingBackend
	}
	if c.Protocol != "vnc" && c.Protocol != "rdp" {
		return nil, core.InvalidConfig
	}
	lib, e := loadLibrary(filepath.Join(dir, libraryName(c.Protocol)))
	if e != nil {
		return nil, core.MissingBackend
	}
	n := &native{lib: lib}
	bind := func(name string, fn any) error {
		sym, e := lib.GetSymbol(name)
		if e != nil {
			return core.MissingBackend
		}
		if ffi.RegisterFunc(fn, sym.Addr()) != nil {
			return core.MissingBackend
		}
		return nil
	}
	var abi func() uint32
	var open func(string, uint32, string, string, string, string, uint32, uint32, uint32, uint32) uintptr
	bindings := []struct {
		name string
		fn   any
	}{{"rcu_abi_version", &abi}, {"rcu_open", &open}, {"rcu_status", &n.status}, {"rcu_snapshot", &n.snapshot}, {"rcu_free", &n.free}, {"rcu_submit", &n.submit}, {"rcu_result", &n.result}, {"rcu_cancel", &n.cancel}, {"rcu_close", &n.close}}
	for _, b := range bindings {
		if bind(b.name, b.fn) != nil {
			lib.Close()
			return nil, core.MissingBackend
		}
	}
	if abi() != 2 {
		lib.Close()
		return nil, core.MissingBackend
	}
	plain := uint32(0)
	if c.AllowUnencrypted {
		plain = 1
	}
	skipVerify := uint32(0)
	if c.SkipTLSVerification() {
		skipVerify = 1
	}
	n.handle = open(c.Host, uint32(c.Port), c.Username, c.Password, c.Domain, c.Fingerprint, uint32(c.Width), uint32(c.Height), plain, skipVerify)
	if n.handle == 0 {
		lib.Close()
		return nil, core.ConnectFailed
	}
	return n, nil
}

// Check loads each adapter and its dependencies without opening a connection
// or reading configuration. Callers expose only our public error codes.
func Check(protocol string) error {
	if !supportedPlatform() {
		return core.Unsupported
	}
	if protocol != "vnc" && protocol != "rdp" {
		return core.InvalidArgs
	}
	dir, err := runtimebundle.Directory()
	if err != nil {
		return core.MissingBackend
	}
	lib, err := loadLibrary(filepath.Join(dir, libraryName(protocol)))
	if err != nil {
		return core.MissingBackend
	}
	defer lib.Close()
	for _, name := range []string{"rcu_open", "rcu_status", "rcu_snapshot", "rcu_free", "rcu_submit", "rcu_result", "rcu_cancel", "rcu_close"} {
		if _, err := lib.GetSymbol(name); err != nil {
			return core.MissingBackend
		}
	}
	sym, err := lib.GetSymbol("rcu_abi_version")
	if err != nil {
		return core.MissingBackend
	}
	var abi func() uint32
	if ffi.RegisterFunc(&abi, sym.Addr()) != nil || abi() != 2 {
		return core.MissingBackend
	}
	return nil
}

func nativeError(n int32) error {
	switch n {
	case -1:
		return core.ConnectFailed
	case -2:
		return core.AuthFailed
	case -3:
		return core.TLSFailed
	case -4:
		return core.Unsupported
	case -5:
		return core.NetworkError
	case -6:
		return core.NotReady
	case -7:
		return core.InvalidArgs
	default:
		return core.InternalError
	}
}

func (n *native) Info() (core.Info, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.handle == 0 {
		return core.Info{}, core.NotConnected
	}
	var v [5]uint64
	state := n.status(n.handle, &v)
	if state < 0 {
		return core.Info{}, nativeError(state)
	}
	s := "connecting"
	if state == 1 {
		s = "ready"
	}
	return core.Info{State: s, Width: int(v[0]), Height: int(v[1]), Sequence: v[2], Geometry: v[3], Capabilities: []string{"screenshot", "pointer", "keyboard", "text-keys"}}, nil
}
func (n *native) Snapshot() (*core.Frame, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.handle == 0 {
		return nil, core.NotConnected
	}
	var v [5]uint64
	p := n.snapshot(n.handle, &v)
	if p == nil {
		state := n.status(n.handle, &v)
		if state < 0 {
			return nil, nativeError(state)
		}
		return nil, core.NotReady
	}
	defer n.free(p)
	if v[0] == 0 || v[1] == 0 || v[0] > 8192 || v[1] > 8192 || v[0]*v[1] > 16<<20 || v[4] != v[0]*v[1]*4 {
		return nil, core.InternalError
	}
	data := append([]byte(nil), unsafe.Slice((*byte)(p), int(v[4]))...)
	return &core.Frame{Width: int(v[0]), Height: int(v[1]), Sequence: v[2], Geometry: v[3], Pixels: data}, nil
}
func (n *native) Send(ctx context.Context, e core.Event) error {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.handle == 0 {
		return core.NotConnected
	}
	id := n.submit(n.handle, int32(e.Kind), int32(e.A), int32(e.B), int32(e.C), int32(e.D))
	if id == 0 {
		return core.NotReady
	}
	t := time.NewTicker(2 * time.Millisecond)
	defer t.Stop()
	for {
		v := n.result(n.handle, id)
		if v == 1 {
			return nil
		}
		if v < 0 {
			return nativeError(v)
		}
		select {
		case <-ctx.Done():
			n.cancel(n.handle, id)
			if ctx.Err() == context.DeadlineExceeded {
				return core.TimedOut
			}
			return core.Canceled
		case <-t.C:
		}
	}
}
func (n *native) Close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.handle != 0 {
		n.close(n.handle)
		n.handle = 0
		n.lib.Close()
	}
}

func supportedPlatform() bool {
	return (runtime.GOOS == "linux" && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64")) || (runtime.GOOS == "darwin" && runtime.GOARCH == "arm64") || (runtime.GOOS == "windows" && runtime.GOARCH == "amd64")
}
func libraryName(protocol string) string {
	ext := ".so"
	if runtime.GOOS == "darwin" {
		ext = ".dylib"
	}
	if runtime.GOOS == "windows" {
		ext = ".dll"
	}
	return "libremote-computer-" + protocol + ext
}
