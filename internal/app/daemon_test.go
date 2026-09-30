package app

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/xuges/remote-computer-use/internal/core"
)

func TestInputWatchdogSerializesWithActions(t *testing.T) {
	b := &fakeBackend{}
	s := &server{gate: make(chan struct{}, 1), control: controller{b: b}}
	if e := s.control.perform(context.Background(), core.Action{Kind: "pointer-down", X: 10, Y: 20}); e != nil {
		t.Fatal(e)
	}
	due := s.control.releaseAt
	s.gate <- struct{}{}
	s.expireInput(due)
	if !s.control.holding() {
		t.Fatal("watchdog interrupted an active action")
	}
	<-s.gate
	s.expireInput(due)
	if s.control.holding() || len(b.events) != 2 || b.events[1].C != 0 {
		t.Fatalf("watchdog failed to release: %+v", b.events)
	}
}

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
				l, err := listenLocal(dir, "test")
				if err != nil {
					t.Fatal(err)
				}
				defer l.Close()
				srv := &http.Server{Handler: rpcHandler(l.endpoint, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_ = json.NewEncoder(w).Encode(core.Result{OK: true, Info: &core.Info{Name: "test", State: "stopped"}})
				}))}
				defer srv.Close()
				go srv.Serve(l)
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
