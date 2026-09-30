package app

import (
	"bytes"
	"encoding/json"
	"github.com/xuges/remote-computer-use/internal/core"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestHTTPRoundtripAndAuthorization(t *testing.T) {
	dir := t.TempDir()
	l, e := listenLocal(dir, "test")
	if e != nil {
		t.Fatal(e)
	}
	data := bytes.Repeat([]byte{0, 255, 13, 10, 128}, 250000)
	srv := &http.Server{Handler: rpcHandler(l.endpoint, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(core.Result{OK: true, PNG: data})
	}))}
	go srv.Serve(l)
	defer srv.Close()
	result := exchange(dir, "test", core.Request{Operation: "status", Timeout: time.Second})
	if !result.OK || !bytes.Equal(result.PNG, data) {
		t.Fatalf("binary HTTP roundtrip failed: %s", result.Error)
	}
	for _, tc := range []struct{ token, origin, host string }{
		{"", "", ""}, {"incorrect", "", ""}, {l.endpoint.Token, "https://example.org", ""}, {l.endpoint.Token, "", "example.org"},
	} {
		req, _ := http.NewRequest("POST", "http://127.0.0.1:"+strconv.Itoa(l.endpoint.Port)+rpcPath, bytes.NewBufferString(`{}`))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", tc.origin)
		if tc.host != "" {
			req.Host = tc.host
		}
		response, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatal("unauthorized HTTP request accepted")
		}
	}
}
func TestHTTPRejectsInvalidRequest(t *testing.T) {
	s := &server{}
	for _, body := range []string{"{", `{"operation":"invalid"}`, `{"operation":"status"} {}`} {
		req := httptest.NewRequest("POST", rpcPath, bytes.NewBufferString(body))
		out := httptest.NewRecorder()
		s.handle(out, req)
		if out.Code == 200 {
			var r core.Result
			if json.Unmarshal(out.Body.Bytes(), &r) != nil || r.OK || r.Error != core.InvalidArgs {
				t.Fatal("invalid request accepted")
			}
		}
	}
}
