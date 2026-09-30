package app

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/xuges/remote-computer-use/internal/backend"
	"github.com/xuges/remote-computer-use/internal/config"
	"github.com/xuges/remote-computer-use/internal/core"
	"image"
	"image/png"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type startup struct {
	Name    string
	Config  config.Private
	Timeout time.Duration
}

func saveInfo(dir string, i core.Info) {
	b, _ := json.Marshal(i)
	p := filepath.Join(dir, i.Name+".json")
	f, e := os.CreateTemp(dir, ".status-")
	if e != nil {
		return
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, e = f.Write(b); e != nil {
		f.Close()
		return
	}
	if f.Close() != nil {
		return
	}
	_ = os.Rename(tmp, p)
}

func query(dir, name, protocol string) core.Info {
	r := exchange(dir, name, core.Request{Operation: "status", Timeout: time.Second})
	if r.OK && r.Info != nil {
		return *r.Info
	}
	i := core.Info{Name: name, Protocol: protocol, State: "stopped"}
	b, e := os.ReadFile(filepath.Join(dir, name+".json"))
	if e == nil && len(b) < 65536 {
		var saved core.Info
		if json.Unmarshal(b, &saved) == nil && saved.Name == name && saved.State == "failed" {
			return saved
		}
	}
	return i
}

func connect(dir, name string, c config.Private, timeout time.Duration) core.Result {
	lock, e := lockFile(filepath.Join(dir, name+".start.lock"))
	if e != nil {
		return core.Result{Error: core.Code(e)}
	}
	defer lock.Close()
	i := query(dir, name, c.Protocol)
	if i.State == "ready" {
		return core.Result{OK: true, Info: &i}
	}
	if i.State == "failed" {
		_ = stop(dir, name)
	}
	c, e = c.Resolve()
	if e != nil {
		return core.Result{Error: core.Code(e), Info: &i}
	}
	exe, e := os.Executable()
	if e != nil {
		return core.Result{Error: core.IOError}
	}
	cmd := exec.Command(exe, "--worker")
	detach(cmd)
	data, _ := json.Marshal(startup{Name: name, Config: c, Timeout: timeout})
	cmd.Stdin = bytes.NewReader(data)
	devnull, e := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if e != nil {
		return core.Result{Error: core.IOError}
	}
	defer devnull.Close()
	cmd.Stderr = devnull
	out, e := cmd.StdoutPipe()
	if e != nil {
		return core.Result{Error: core.IOError}
	}
	if cmd.Start() != nil {
		return core.Result{Error: core.IOError}
	}
	ch := make(chan core.Result, 1)
	go func() {
		var r core.Result
		if json.NewDecoder(io.LimitReader(out, 1<<20)).Decode(&r) != nil {
			r = core.Result{Error: core.ConnectFailed}
		}
		ch <- r
	}()
	select {
	case r := <-ch:
		out.Close()
		if !r.OK {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		} else {
			_ = cmd.Process.Release()
		}
		return r
	case <-time.After(timeout + 2*time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		out.Close()
		i.State = "failed"
		i.Error = core.TimedOut
		saveInfo(dir, i)
		return core.Result{Error: core.TimedOut, Info: &i}
	}
}

func stop(dir, name string) core.Result {
	r := exchange(dir, name, core.Request{Operation: "disconnect", Timeout: 5 * time.Second})
	if r.Error == core.NotConnected {
		i := core.Info{Name: name, State: "stopped"}
		r = core.Result{OK: true, Info: &i}
	}
	if !r.OK {
		return r
	}
	for j := 0; j < 100; j++ {
		// This lifetime lock covers both the listener and native cleanup,
		// including named pipes which have no filesystem socket to inspect.
		lock, e := lockFile(filepath.Join(dir, name+".lock"))
		if e == nil {
			saveInfo(dir, *r.Info)
			lock.Close()
			return r
		}
		if e != core.NotReady {
			return core.Result{Error: core.Code(e)}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return core.Result{Error: core.TimedOut}
}

type server struct {
	name, protocol, dir string
	b                   backend.Backend
	mu                  sync.Mutex
	busy                bool
	state               string
	errorCode           core.Error
	gate                chan struct{}
	control             controller
	ctx                 context.Context
	cancel              context.CancelFunc
}

func (s *server) info() core.Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := core.Info{Name: s.name, Protocol: s.protocol, State: s.state, Busy: s.busy, Error: s.errorCode}
	if s.b != nil && s.state != "disconnecting" {
		v, e := s.b.Info()
		if e != nil {
			i.State = "failed"
			i.Error = core.Code(e)
		} else {
			i.State = v.State
			i.Width = v.Width
			i.Height = v.Height
			i.Geometry = v.Geometry
			i.Sequence = v.Sequence
			i.Capabilities = v.Capabilities
		}
	}
	return i
}

func (s *server) handle(w http.ResponseWriter, request *http.Request) {
	var req core.Request
	request.Body = http.MaxBytesReader(w, request.Body, maxPacket)
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Second))
	decoder := json.NewDecoder(request.Body)
	if decoder.Decode(&req) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Time{})
	if req.Operation == "status" {
		i := s.info()
		_ = json.NewEncoder(w).Encode(core.Result{OK: true, Info: &i})
		return
	}
	if req.Operation == "disconnect" {
		s.mu.Lock()
		s.state = "disconnecting"
		s.mu.Unlock()
		i := core.Info{Name: s.name, Protocol: s.protocol, State: "stopped"}
		_ = json.NewEncoder(w).Encode(core.Result{OK: true, Info: &i})
		s.cancel()
		return
	}
	if req.Operation != "action" {
		_ = json.NewEncoder(w).Encode(core.Result{Error: core.InvalidArgs})
		return
	}
	if req.Timeout <= 0 || req.Timeout > time.Hour {
		req.Timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(request.Context(), req.Timeout)
	defer cancel()
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		_ = json.NewEncoder(w).Encode(core.Result{Error: core.Code(canceled(ctx))})
		return
	}
	defer func() { <-s.gate }()
	s.mu.Lock()
	b := s.b
	s.busy = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.busy = false; s.mu.Unlock() }()
	res := core.Result{}
	if b == nil {
		res.Error = core.NotReady
	} else if req.Action.Kind == "screenshot" {
		f, e := b.Snapshot()
		if e != nil {
			res.Error = core.Code(e)
		} else {
			var buf bytes.Buffer
			e = png.Encode(&buf, &image.RGBA{Pix: f.Pixels, Stride: f.Width * 4, Rect: image.Rect(0, 0, f.Width, f.Height)})
			if e != nil {
				res.Error = core.InternalError
			} else {
				res = core.Result{OK: true, PNG: buf.Bytes(), Width: f.Width, Height: f.Height, Geometry: f.Geometry, Sequence: f.Sequence}
			}
		}
	} else {
		if e := s.control.perform(ctx, req.Action); e != nil {
			res.Error = core.Code(e)
		} else {
			res.OK = true
		}
	}
	if ctx.Err() != nil {
		res = core.Result{Error: core.Code(canceled(ctx))}
	}
	if !res.OK {
		_ = s.control.releaseInput()
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
	if json.NewEncoder(w).Encode(res) != nil {
		_ = s.control.releaseInput()
	}
}

// Called by the worker, never concurrently with input actions. Status and
// screenshots do not renew the lease. A busy action owns its own cleanup.
func (s *server) expireInput(now time.Time) {
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
		_ = s.control.expireInput(now)
	default:
	}
}

func Worker() int {
	var start startup
	if json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&start) != nil || !config.ValidName(start.Name) {
		return 1
	}
	// Preserve a private ready pipe, then silence native stdout/stderr before loading libraries.
	ready, e := startupPipe()
	if e != nil {
		return 1
	}
	defer ready.Close()
	fail := func(code core.Error) int { _ = json.NewEncoder(ready).Encode(core.Result{Error: code}); return 1 }
	dir, e := runtimeDir()
	if e != nil {
		return fail(core.Code(e))
	}
	lock, e := lockFile(filepath.Join(dir, start.Name+".lock"))
	if e != nil {
		return fail(core.Code(e))
	}
	defer lock.Close()
	l, e := listenLocal(dir, start.Name)
	if e != nil {
		return fail(core.IOError)
	}
	defer l.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	s := &server{name: start.Name, protocol: start.Config.Protocol, dir: dir, state: "connecting", gate: make(chan struct{}, 1), ctx: ctx, cancel: cancel}
	httpServer := &http.Server{Handler: rpcHandler(l.endpoint, http.HandlerFunc(s.handle)), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8192, ErrorLog: log.New(io.Discard, "", 0), BaseContext: func(net.Listener) context.Context { return ctx }}
	accepted := make(chan struct{})
	go func() { defer close(accepted); _ = httpServer.Serve(l) }()
	shutdown := func() {
		cancel()
		shutdownCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if httpServer.Shutdown(shutdownCtx) != nil {
			_ = httpServer.Close()
		}
		<-accepted
	}

	b, e := backend.Open(start.Config)
	if e != nil {
		shutdown()
		i := s.info()
		i.State = "failed"
		i.Error = core.Code(e)
		saveInfo(dir, i)
		return fail(core.Code(e))
	}
	s.mu.Lock()
	s.b = b
	s.control.b = b
	s.mu.Unlock()
	defer b.Close()
	timeout := start.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		i := s.info()
		if i.State == "ready" {
			saveInfo(dir, i)
			_ = json.NewEncoder(ready).Encode(core.Result{OK: true, Info: &i})
			ready.Close()
			break
		}
		if i.State == "failed" || time.Now().After(deadline) || ctx.Err() != nil {
			if i.Error == "" {
				i.Error = core.TimedOut
			}
			i.State = "failed"
			saveInfo(dir, i)
			shutdown()
			return fail(i.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
running:
	for {
		select {
		case <-ctx.Done():
			break running
		case now := <-ticker.C:
			s.expireInput(now)
		}
	}
	shutdown()
	s.gate <- struct{}{}
	_ = s.control.releaseInput()
	<-s.gate
	i := core.Info{Name: start.Name, Protocol: start.Config.Protocol, State: "stopped"}
	saveInfo(dir, i)
	return 0
}
