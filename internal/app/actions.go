package app

import (
	"context"
	"github.com/xuges/remote-computer-use/internal/backend"
	"github.com/xuges/remote-computer-use/internal/core"
	"strings"
	"time"
	"unicode/utf8"
)

type controller struct {
	b            backend.Backend
	x, y         int
	buttons      int
	keys         []core.Event
	releaseAt    time.Time
	heldGeometry uint64
}

// The worker owns this state under its action gate. A completed CLI process
// must not release explicit holds, so the worker supplies a bounded idle lease.
const inputIdleTimeout = 30 * time.Second

func (c *controller) holding() bool { return c.buttons != 0 || len(c.keys) != 0 }

func (c *controller) keyHeld(key core.Event) bool {
	for _, v := range c.keys {
		if v.A == key.A {
			return true
		}
	}
	return false
}

func (c *controller) forgetKey(key core.Event) {
	for j, v := range c.keys {
		if v.A == key.A {
			c.keys = append(c.keys[:j], c.keys[j+1:]...)
			return
		}
	}
}

// Release bypasses cancellation and stale geometry guards. Retain failures so
// the watchdog can retry; a failed send may already have reached the server.
func (c *controller) releaseInput() (err error) {
	c.releaseAt = time.Time{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if c.buttons != 0 {
		if i, e := c.b.Info(); e == nil && i.Width > 0 && i.Height > 0 {
			c.x = max(0, min(c.x, i.Width-1))
			c.y = max(0, min(c.y, i.Height-1))
		}
		if e := c.b.Send(ctx, core.Event{Kind: core.Pointer, A: c.x, B: c.y}); e != nil {
			err = e
		} else {
			c.buttons = 0
		}
	}
	for j := len(c.keys) - 1; j >= 0; j-- {
		v := c.keys[j]
		v.C = 0
		if e := c.b.Send(ctx, v); e != nil {
			if err == nil {
				err = e
			}
		} else {
			c.forgetKey(v)
		}
	}
	return err
}

func (c *controller) expireInput(now time.Time) error {
	if !c.holding() {
		return nil
	}
	i, e := c.b.Info()
	if e != nil || i.State != "ready" || i.Geometry != c.heldGeometry || !now.Before(c.releaseAt) {
		return c.releaseInput()
	}
	return nil
}

func (c *controller) perform(ctx context.Context, a core.Action) (err error) {
	if a.Kind == "release-input" {
		return c.releaseInput()
	}
	if e := c.expireInput(time.Now()); e != nil {
		return e
	}
	defer func() {
		if err != nil {
			_ = c.releaseInput()
		} else if c.holding() {
			c.releaseAt = time.Now().Add(inputIdleTimeout)
		}
	}()
	return c.execute(ctx, a)
}

func canceled(ctx context.Context) error {
	if ctx.Err() == context.DeadlineExceeded {
		return core.TimedOut
	}
	return core.Canceled
}
func pause(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return canceled(ctx)
	case <-t.C:
		return nil
	}
}

func (c *controller) execute(ctx context.Context, a core.Action) (err error) {
	i, e := c.b.Info()
	if e != nil {
		return e
	}
	if i.State != "ready" {
		return core.NotReady
	}
	if a.Geometry != 0 && a.Geometry != i.Geometry {
		return core.GeometryChanged
	}
	g := i.Geometry
	c.heldGeometry = g
	send := func(v core.Event) error {
		if ctx.Err() != nil {
			return canceled(ctx)
		}
		current, e := c.b.Info()
		if e != nil {
			return e
		}
		if current.Geometry != g {
			return core.GeometryChanged
		}
		if current.State != "ready" {
			return core.NotReady
		}
		return c.b.Send(ctx, v)
	}
	point := func(x, y int) bool { return x >= 0 && y >= 0 && x < i.Width && y < i.Height }
	var held []core.Event
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		for j := len(held) - 1; j >= 0; j-- {
			v := held[j]
			v.C = 0
			if v.Kind == core.Pointer {
				v.C = c.buttons
			}
			if e := c.b.Send(cleanup, v); e != nil {
				if v.Kind == core.Key && !c.keyHeld(v) {
					c.keys = append(c.keys, v)
				}
				if v.Kind == core.Pointer {
					c.buttons |= buttonMask(a.Button)
				}
				if err == nil {
					err = e
				}
			}
		}
	}()
	move := func(x, y, mask int) error {
		if !point(x, y) {
			return core.InvalidArgs
		}
		e := send(core.Event{Kind: core.Pointer, A: x, B: y, C: mask})
		if e == nil {
			c.x, c.y = x, y
		}
		return e
	}
	switch a.Kind {
	case "move":
		return move(a.X, a.Y, c.buttons)
	case "pointer-down", "pointer-up":
		mask := buttonMask(a.Button)
		if mask == 0 {
			return core.InvalidArgs
		}
		if a.Kind == "pointer-up" {
			if c.buttons&mask == 0 {
				return nil
			}
			if e := move(c.x, c.y, c.buttons&^mask); e != nil {
				return e
			}
			c.buttons &^= mask
			return nil
		}
		if !point(a.X, a.Y) {
			return core.InvalidArgs
		}
		c.buttons |= mask // Track even an ambiguous failed down for cleanup.
		c.x, c.y = a.X, a.Y
		return move(a.X, a.Y, c.buttons)
	case "click", "drag":
		mask := buttonMask(a.Button)
		if mask == 0 || c.buttons != 0 {
			return core.InvalidArgs
		}
		if !point(a.X, a.Y) {
			return core.InvalidArgs
		}
		if a.Kind == "drag" && (!point(a.ToX, a.ToY) || a.Duration <= 0 || a.Duration > time.Minute) {
			return core.InvalidArgs
		}
		count := a.Count
		if count == 0 {
			count = 1
		}
		if count < 1 || count > 2 {
			return core.InvalidArgs
		}
		if a.Kind == "drag" {
			count = 1
		}
		for j := 0; j < count; j++ {
			if e := move(a.X, a.Y, 0); e != nil {
				return e
			}
			held = []core.Event{{Kind: core.Pointer, A: a.X, B: a.Y}}
			if e := move(a.X, a.Y, mask); e != nil {
				return e
			}
			if a.Kind == "drag" {
				steps := int(a.Duration / (20 * time.Millisecond))
				if steps < 1 {
					steps = 1
				}
				if steps > 300 {
					steps = 300
				}
				for s := 1; s <= steps; s++ {
					if e := pause(ctx, a.Duration/time.Duration(steps)); e != nil {
						return e
					}
					x := a.X + (a.ToX-a.X)*s/steps
					y := a.Y + (a.ToY-a.Y)*s/steps
					held[0].A, held[0].B = x, y
					if e := move(x, y, mask); e != nil {
						return e
					}
				}
			}
			if e := move(c.x, c.y, 0); e != nil {
				return e
			}
			held = nil
			if j+1 < count {
				if e := pause(ctx, 100*time.Millisecond); e != nil {
					return e
				}
			}
		}
		return nil
	case "scroll":
		// VNC wheel events contain a complete button mask. Do not silently
		// release a held button (the native wheel ABI has no mask argument).
		if c.buttons != 0 {
			return core.InvalidArgs
		}
		if a.DX < -100 || a.DX > 100 || a.DY < -100 || a.DY > 100 || (a.DX == 0 && a.DY == 0) {
			return core.InvalidArgs
		}
		return send(core.Event{Kind: core.Wheel, A: a.DX, B: a.DY, C: c.x, D: c.y})
	case "key-down", "key-up":
		keys, e := parseChord(a.Chord)
		if e != nil || len(keys) != 1 {
			return core.InvalidArgs
		}
		v := keys[0]
		if a.Kind == "key-up" {
			if !c.keyHeld(v) {
				return nil
			}
			if e := send(v); e != nil {
				return e
			}
			c.forgetKey(v)
			return nil
		}
		if c.keyHeld(v) {
			return nil
		}
		c.keys = append(c.keys, v)
		v.C = 1
		return send(v)
	case "key":
		keys, e := parseChord(a.Chord)
		if e != nil {
			return e
		}
		for _, v := range keys {
			if c.keyHeld(v) {
				continue
			}
			held = append(held, v)
			v.C = 1
			if e := send(v); e != nil {
				return e
			}
		}
		return nil
	case "type":
		if len(c.keys) != 0 {
			return core.InvalidArgs
		}
		if a.Method != "" && a.Method != "keys" {
			return core.Unsupported
		}
		if len(a.Text) > 1<<20 || !utf8.ValidString(a.Text) || strings.ContainsRune(a.Text, 0) {
			return core.InvalidArgs
		}
		for _, r := range a.Text {
			if r < 32 && r != '\n' && r != '\t' && r != '\r' {
				return core.InvalidArgs
			}
		}
		for _, r := range a.Text {
			if r == '\n' || r == '\r' || r == '\t' {
				key := "ENTER"
				if r == '\t' {
					key = "TAB"
				}
				ks, _ := parseChord(key)
				v := ks[0]
				held = []core.Event{v}
				v.C = 1
				if e := send(v); e != nil {
					return e
				}
				v.C = 0
				if e := send(v); e != nil {
					return e
				}
				held = nil
			} else {
				if e := send(core.Event{Kind: core.Unicode, A: int(r)}); e != nil {
					return e
				}
			}
		}
		return nil
	default:
		return core.InvalidArgs
	}
}

func buttonMask(button string) int {
	switch button {
	case "", "left":
		return 1
	case "middle":
		return 2
	case "right":
		return 4
	default:
		return 0
	}
}

func parseChord(s string) ([]core.Event, error) {
	parts := strings.Split(strings.ToUpper(s), "+")
	if len(parts) > 12 {
		return nil, core.InvalidArgs
	}
	m := map[string][2]int{"CTRL": {0xffe3, 0x1d}, "CONTROL": {0xffe3, 0x1d}, "SHIFT": {0xffe1, 0x2a}, "ALT": {0xffe9, 0x38}, "CMD": {0xffeb, 0x15b}, "WIN": {0xffeb, 0x15b}, "SUPER": {0xffeb, 0x15b}, "ENTER": {0xff0d, 0x1c}, "TAB": {0xff09, 0xf}, "ESC": {0xff1b, 1}, "ESCAPE": {0xff1b, 1}, "BACKSPACE": {0xff08, 0xe}, "DELETE": {0xffff, 0x153}, "SPACE": {32, 0x39}, "LEFT": {0xff51, 0x14b}, "UP": {0xff52, 0x148}, "RIGHT": {0xff53, 0x14d}, "DOWN": {0xff54, 0x150}, "HOME": {0xff50, 0x147}, "END": {0xff57, 0x14f}, "PAGEUP": {0xff55, 0x149}, "PAGEDOWN": {0xff56, 0x151}, "INSERT": {0xff63, 0x152}, "MINUS": {45, 0xc}, "EQUAL": {61, 0xd}}
	letters := "QWERTYUIOPASDFGHJKLZXCVBNM"
	scans := []int{0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1e, 0x1f, 0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x2c, 0x2d, 0x2e, 0x2f, 0x30, 0x31, 0x32}
	for j, ch := range letters {
		m[string(ch)] = [2]int{int(ch + 32), scans[j]}
	}
	for j, ch := range "1234567890" {
		m[string(ch)] = [2]int{int(ch), j + 2}
	}
	for j, name := range []string{"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12"} {
		scan := 0x3b + j
		if j >= 10 {
			scan = 0x57 + j - 10
		}
		m[name] = [2]int{0xffbe + j, scan}
	}
	var out []core.Event
	seen := map[int]bool{}
	for _, part := range parts {
		v, ok := m[part]
		if !ok || seen[v[0]] {
			return nil, core.InvalidArgs
		}
		seen[v[0]] = true
		out = append(out, core.Event{Kind: core.Key, A: v[0], B: v[1]})
	}
	return out, nil
}
