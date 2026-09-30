package app

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xuges/remote-computer-use/internal/core"
	"github.com/xuges/remote-computer-use/internal/host"
)

const maxPacket = 128 << 20
const rpcPath = "/v1/rpc"

type endpoint struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
}
type localListener struct {
	net.Listener
	endpoint endpoint
	path     string
	once     sync.Once
}

func (l *localListener) Close() error {
	err := l.Listener.Close()
	l.once.Do(func() { _ = os.Remove(l.path) })
	return err
}
func endpointPath(dir, name string) string { return filepath.Join(dir, name+".endpoint.json") }
func listenLocal(dir, name string) (*localListener, error) {
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return nil, e
	}
	token := make([]byte, 32)
	if _, e = rand.Read(token); e != nil {
		l.Close()
		return nil, e
	}
	ep := endpoint{Port: l.Addr().(*net.TCPAddr).Port, Token: hex.EncodeToString(token)}
	data, _ := json.Marshal(ep)
	f, e := os.CreateTemp(dir, ".endpoint-")
	if e != nil {
		l.Close()
		return nil, e
	}
	defer os.Remove(f.Name())
	if e = host.Protect(f.Name(), false); e == nil {
		_, e = f.Write(data)
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	path := endpointPath(dir, name)
	if e == nil {
		e = os.Rename(f.Name(), path)
	}
	if e != nil {
		l.Close()
		return nil, e
	}
	return &localListener{Listener: l, endpoint: ep, path: path}, nil
}
func readEndpoint(dir, name string) (endpoint, error) {
	var ep endpoint
	path := endpointPath(dir, name)
	if e := host.CheckPrivate(path, false); e != nil {
		return ep, core.NotConnected
	}
	f, e := os.Open(path)
	if e != nil {
		return ep, core.NotConnected
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, 4097))
	if e != nil || len(data) > 4096 || json.Unmarshal(data, &ep) != nil || ep.Port < 1 || ep.Port > 65535 {
		return ep, core.NotConnected
	}
	token, e := hex.DecodeString(ep.Token)
	if e != nil || len(token) != 32 {
		return ep, core.NotConnected
	}
	return ep, nil
}
func rpcHandler(ep endpoint, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fixed loopback Host and no browser-origin API prevent DNS rebinding/CSRF.
		wantHost := net.JoinHostPort("127.0.0.1", strconv.Itoa(ep.Port))
		auth := r.Header.Get("Authorization")
		if r.Host != wantHost || r.Header.Get("Origin") != "" || !strings.HasPrefix(auth, "Bearer ") || subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, "Bearer ")), []byte(ep.Token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != rpcPath {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
func exchange(dir, name string, req core.Request) core.Result {
	ep, e := readEndpoint(dir, name)
	if e != nil {
		return core.Result{Error: core.NotConnected}
	}
	data, e := json.Marshal(req)
	if e != nil || len(data) > maxPacket {
		return core.Result{Error: core.InvalidArgs}
	}
	d := req.Timeout
	if d <= 0 {
		d = 10 * time.Second
	}
	// No proxy, redirects or remote addresses: an environment HTTP_PROXY must
	// never receive local authorization tokens, screenshots or typed text.
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: d + 2*time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+strconv.Itoa(ep.Port)+rpcPath, bytes.NewReader(data))
	request.Header.Set("Authorization", "Bearer "+ep.Token)
	request.Header.Set("Content-Type", "application/json")
	response, e := client.Do(request)
	if e != nil {
		return core.Result{Error: core.NotConnected}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return core.Result{Error: core.NetworkError}
	}
	body, e := io.ReadAll(io.LimitReader(response.Body, maxPacket+1))
	if e != nil || len(body) > maxPacket {
		return core.Result{Error: core.NetworkError}
	}
	var result core.Result
	if json.Unmarshal(body, &result) != nil {
		return core.Result{Error: core.NetworkError}
	}
	return result
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
