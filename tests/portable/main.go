// Portable CLI/native VNC smoke test. The RFB fixture is an isolated in-memory
// desktop: keyboard and mouse tests never touch the user's actual desktop.
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"image/png"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/xuges/remote-computer-use/internal/host"
)

type event struct {
	kind byte
	down bool
	key  uint32
	mask byte
	x, y uint16
}
type desktop struct {
	mu     sync.Mutex
	events []event
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func read(c net.Conn, n int) []byte { b := make([]byte, n); mustRead(c, b); return b }
func mustRead(c net.Conn, b []byte) {
	if _, e := io.ReadFull(c, b); e != nil {
		panic(e)
	}
}
func (d *desktop) serve(c net.Conn) {
	defer c.Close()
	defer func() { _ = recover() }()
	c.SetDeadline(time.Now().Add(90 * time.Second))
	c.Write([]byte("RFB 003.008\n"))
	read(c, 12)
	c.Write([]byte{1, 1})
	read(c, 1)
	c.Write([]byte{0, 0, 0, 0})
	read(c, 1)
	init := new(bytes.Buffer)
	binary.Write(init, binary.BigEndian, uint16(640))
	binary.Write(init, binary.BigEndian, uint16(480))
	init.Write([]byte{32, 24, 0, 1, 0, 255, 0, 255, 0, 255, 16, 8, 0, 0, 0, 0})
	binary.Write(init, binary.BigEndian, uint32(7))
	init.WriteString("fixture")
	c.Write(init.Bytes())
	format := []byte{32, 24, 0, 1, 0, 255, 0, 255, 0, 255, 16, 8, 0, 0, 0, 0}
	for {
		kind := read(c, 1)[0]
		switch kind {
		case 0:
			read(c, 3)
			format = read(c, 16)
		case 2:
			h := read(c, 3)
			read(c, int(binary.BigEndian.Uint16(h[1:]))*4)
		case 3:
			read(c, 9)
			time.Sleep(20 * time.Millisecond)
			if format[0] != 32 {
				panic("fixture requires 32-bit pixels")
			}
			frame := make([]byte, 16+640*480*4)
			frame[3] = 1
			binary.BigEndian.PutUint16(frame[8:], 640)
			binary.BigEndian.PutUint16(frame[10:], 480)
			pixel := uint32(binary.BigEndian.Uint16(format[4:6])) << format[10]
			for i := 16; i < len(frame); i += 4 {
				if format[2] == 0 {
					binary.LittleEndian.PutUint32(frame[i:], pixel)
				} else {
					binary.BigEndian.PutUint32(frame[i:], pixel)
				}
			}
			if _, e := c.Write(frame); e != nil {
				return
			}
		case 4:
			b := read(c, 7)
			d.mu.Lock()
			d.events = append(d.events, event{kind: 4, down: b[0] != 0, key: binary.BigEndian.Uint32(b[3:])})
			d.mu.Unlock()
		case 5:
			b := read(c, 5)
			d.mu.Lock()
			d.events = append(d.events, event{kind: 5, mask: b[0], x: binary.BigEndian.Uint16(b[1:]), y: binary.BigEndian.Uint16(b[3:])})
			d.mu.Unlock()
		case 6:
			h := read(c, 7)
			read(c, int(binary.BigEndian.Uint32(h[3:])))
		default:
			panic(fmt.Sprintf("unexpected RFB message %d", kind))
		}
	}
}
func (d *desktop) has(want event) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.events {
		if e == want {
			return true
		}
	}
	return false
}
func main() {
	bin := flag.String("bin", "bin", "directory containing the two CLI binaries")
	flag.Parse()
	root, e := filepath.Abs(*bin)
	must(e)
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	ctl, use := filepath.Join(root, "remote-computer-ctl"+ext), filepath.Join(root, "remote-computer-use"+ext)
	tmp, e := os.MkdirTemp("", "rcu portable 中文 ")
	must(e)
	defer os.RemoveAll(tmp)
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	must(e)
	defer l.Close()
	d := new(desktop)
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go d.serve(c)
		}
	}()
	config := filepath.Join(tmp, "config.toml")
	must(os.WriteFile(config, []byte(fmt.Sprintf("version = 1\n[connections.fixture]\nprotocol = 'vnc'\nhost = '127.0.0.1'\nport = %d\nallow_unencrypted = true\n", l.Addr().(*net.TCPAddr).Port)), 0600))
	must(host.Protect(config, false))
	env := os.Environ()
	// Setenv updates inherited settings without duplicate environment keys.
	os.Setenv("REMOTE_COMPUTER_CONFIG", config)
	os.Setenv("REMOTE_COMPUTER_RUN_DIR", filepath.Join(tmp, strings.Repeat("long-", 20), "run"))
	env = os.Environ()
	call := func(binary string, args ...string) []byte {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env = env
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if e := cmd.Run(); e != nil {
			panic(fmt.Sprintf("%s: %v %s %s", args, e, stdout.String(), stderr.String()))
		}
		return stdout.Bytes()
	}
	call(ctl, "doctor", "--json")
	call(ctl, "connect", "fixture", "--json")
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, ctl, "disconnect", "fixture")
		cmd.Env = env
		cmd.Run()
	}()
	call(ctl, "connect", "fixture", "--json")
	state := call(ctl, "status", "fixture", "--json")
	var info map[string]any
	must(json.Unmarshal(state, &info))
	if info["state"] != "ready" || info["width"] != float64(640) {
		panic("unexpected state")
	}
	for _, key := range []string{"host", "port", "password", "username", "token"} {
		if _, found := info[key]; found {
			panic("private field in status")
		}
	}
	shot := call(use, "--conn", "fixture", "screenshot", "--output", "-")
	im, e := png.Decode(bytes.NewReader(shot))
	must(e)
	r, g, b, _ := im.At(100, 100).RGBA()
	if im.Bounds().Dx() != 640 || r != 65535 || g != 0 || b != 0 {
		panic("screenshot colors or dimensions")
	}
	call(use, "--conn", "fixture", "click", "--x", "123", "--y", "234", "--count", "2")
	call(use, "--conn", "fixture", "key", "CTRL+L")
	call(use, "--conn", "fixture", "type", "--text", "中😀")
	call(use, "--conn", "fixture", "scroll", "--dy", "2", "--dx", "2")
	call(use, "--conn", "fixture", "drag", "--from", "30,40", "--to", "90,100", "--duration", "100ms")
	want := []event{{kind: 4, down: true, key: 0x01004e2d}, {kind: 4, key: 0x01004e2d}, {kind: 4, down: true, key: 0x0101f600}, {kind: 4, key: 0x0101f600}, {kind: 4, down: true, key: 65507}, {kind: 4, key: 65507}, {kind: 5, mask: 1, x: 123, y: 234}, {kind: 5, x: 90, y: 100}}
	for _, w := range want {
		for n := 0; n < 100 && !d.has(w); n++ {
			time.Sleep(10 * time.Millisecond)
		}
		if !d.has(w) {
			panic(fmt.Sprintf("missing input event %+v", w))
		}
	}
	// Terminate a requesting CLI while the worker has a button held.
	dragging := exec.Command(use, "--conn", "fixture", "drag", "--from", "50,60", "--to", "300,300", "--duration", "5s")
	dragging.Env = env
	must(dragging.Start())
	for n := 0; n < 100 && !d.has(event{kind: 5, mask: 1, x: 50, y: 60}); n++ {
		time.Sleep(10 * time.Millisecond)
	}
	must(dragging.Process.Kill())
	dragging.Wait()
	time.Sleep(200 * time.Millisecond)
	d.mu.Lock()
	last := event{}
	for _, e := range d.events {
		if e.kind == 5 {
			last = e
		}
	}
	d.mu.Unlock()
	if last.mask != 0 {
		panic("cancellation left mouse pressed")
	}
	call(ctl, "disconnect", "--all", "--json")
	call(ctl, "connect", "--all", "--json")
	call(ctl, "disconnect", "--all", "--json")
	fmt.Println("PASS: native VNC, HTTP, PNG/colors, Unicode, keyboard/mouse, cancellation, reconnect, long Unicode paths")
}
