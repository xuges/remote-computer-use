package app

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/xuges/remote-computer-use/internal/core"
)

func TestStopWaitsForBackendCleanup(t *testing.T) {
	for _, missingSocket := range []bool{false, true} {
		name := "listener"
		if missingSocket {
			name = "already-unlinked"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			lock, err := lockFile(filepath.Join(dir, "test.lock"))
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			if !missingSocket {
				l, err := net.Listen("unix", socketPath(dir, "test"))
				if err != nil {
					t.Fatal(err)
				}
				defer l.Close()
				go func() {
					c, err := l.Accept()
					if err != nil {
						return
					}
					defer c.Close()
					var req core.Request
					if readPacket(c, &req) != nil {
						return
					}
					_ = writePacket(c, core.Result{OK: true, Info: &core.Info{Name: "test", State: "stopped"}})
					l.Close() // Native backend still holds the worker lock.
				}()
			}
			done := make(chan core.Result, 1)
			go func() { done <- stop(dir, "test") }()
			select {
			case r := <-done:
				t.Fatalf("returned before backend cleanup: %+v", r)
			case <-time.After(100 * time.Millisecond):
			}
			lock.Close()
			select {
			case r := <-done:
				if !r.OK {
					t.Fatalf("stop: %+v", r)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("stop did not finish after cleanup")
			}
		})
	}
}
