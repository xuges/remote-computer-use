package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/xuges/remote-computer-use/internal/core"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeBackend struct {
	events   []core.Event
	failAt   int
	geometry uint64
}

func TestDragWithEscapeAcrossCommands(t *testing.T) {
	b := &fakeBackend{geometry: 1}
	c := controller{b: b}
	for _, a := range []core.Action{
		{Kind: "pointer-down", X: 10, Y: 20},
		{Kind: "move", X: 50, Y: 60},
		{Kind: "key", Chord: "ESC"},
		{Kind: "pointer-up"},
	} {
		if e := c.perform(context.Background(), a); e != nil {
			t.Fatal(e)
		}
	}
	want := []core.Event{
		{Kind: core.Pointer, A: 10, B: 20, C: 1},
		{Kind: core.Pointer, A: 50, B: 60, C: 1},
		{Kind: core.Key, A: 0xff1b, B: 1, C: 1},
		{Kind: core.Key, A: 0xff1b, B: 1},
		{Kind: core.Pointer, A: 50, B: 60},
	}
	if !reflect.DeepEqual(b.events, want) || c.holding() {
		t.Fatalf("events: %+v", b.events)
	}
}

func TestExplicitHoldsPreserveOtherInputs(t *testing.T) {
	b := &fakeBackend{}
	c := controller{b: b}
	run := func(a core.Action) {
		t.Helper()
		if e := c.perform(context.Background(), a); e != nil {
			t.Fatal(e)
		}
	}
	run(core.Action{Kind: "key-down", Chord: "CTRL"})
	run(core.Action{Kind: "key-down", Chord: "CONTROL"}) // alias is idempotent
	run(core.Action{Kind: "key", Chord: "CTRL+A"})
	if len(b.events) != 3 || b.events[2].A != 'a' || b.events[2].C != 0 || len(c.keys) != 1 {
		t.Fatalf("shortcut released the explicit modifier: %+v", b.events)
	}
	run(core.Action{Kind: "pointer-down", X: 1, Y: 2, Button: "left"})
	run(core.Action{Kind: "pointer-down", X: 2, Y: 3, Button: "right"})
	run(core.Action{Kind: "pointer-up", Button: "left"})
	if b.events[len(b.events)-1].C != 4 {
		t.Fatal("released the other button")
	}
	run(core.Action{Kind: "key-up", Chord: "CONTROL"})
	if len(c.keys) != 0 || c.buttons != 4 {
		t.Fatal("key up changed button state")
	}
	run(core.Action{Kind: "release-input", Geometry: 999})
	if c.holding() || b.events[len(b.events)-1].C != 0 {
		t.Fatal("release-input left inputs held")
	}
	count := len(b.events)
	run(core.Action{Kind: "pointer-up", Button: "right"})
	run(core.Action{Kind: "key-up", Chord: "CTRL"})
	run(core.Action{Kind: "release-input"})
	if len(b.events) != count {
		t.Fatal("idempotent cleanup sent extra input")
	}
}

func TestHeldInputCleanup(t *testing.T) {
	for _, reason := range []string{"idle", "resize", "invalid", "cancel", "partial-down", "release-failed"} {
		t.Run(reason, func(t *testing.T) {
			b := &fakeBackend{geometry: 1}
			c := controller{b: b}
			for _, a := range []core.Action{{Kind: "key-down", Chord: "SHIFT"}, {Kind: "pointer-down", X: 20, Y: 30}} {
				if e := c.perform(context.Background(), a); e != nil {
					t.Fatal(e)
				}
			}
			switch reason {
			case "idle":
				if e := c.expireInput(c.releaseAt.Add(-time.Millisecond)); e != nil || !c.holding() {
					t.Fatal("released early")
				}
				if e := c.expireInput(c.releaseAt); e != nil {
					t.Fatal(e)
				}
			case "resize":
				b.geometry++
				_ = c.expireInput(time.Now())
			case "invalid":
				if e := c.perform(context.Background(), core.Action{Kind: "move", X: -1}); e != core.InvalidArgs {
					t.Fatal(e)
				}
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if e := c.perform(ctx, core.Action{Kind: "key", Chord: "ESC"}); e != core.Canceled {
					t.Fatal(e)
				}
			case "partial-down":
				b.failAt = len(b.events) + 1
				if e := c.perform(context.Background(), core.Action{Kind: "key-down", Chord: "ALT"}); e != core.NetworkError {
					t.Fatal(e)
				}
			case "release-failed":
				b.failAt = len(b.events) + 1
				if e := c.releaseInput(); e != core.NetworkError || c.buttons == 0 {
					t.Fatal("lost pending cleanup")
				}
				if e := c.expireInput(time.Now()); e != nil {
					t.Fatal(e)
				}
			}
			if c.holding() {
				t.Fatalf("inputs held after %s", reason)
			}
			var pointerReleased, shiftReleased bool
			for _, e := range b.events[2:] {
				pointerReleased = pointerReleased || (e.Kind == core.Pointer && e.C == 0)
				shiftReleased = shiftReleased || (e.Kind == core.Key && e.A == 0xffe1 && e.C == 0)
			}
			if !pointerReleased || !shiftReleased {
				t.Fatalf("missing releases: %+v", b.events)
			}
		})
	}
}

func TestHoldConflictsAndInvalidKeys(t *testing.T) {
	for _, a := range []core.Action{
		{Kind: "click", X: 1, Y: 1},
		{Kind: "drag", X: 1, Y: 1, ToX: 2, ToY: 2, Duration: time.Second},
		{Kind: "scroll", DY: 1}, {Kind: "type", Text: "hello"},
		{Kind: "key-down", Chord: "CTRL+A"}, {Kind: "key-up", Chord: "bad"},
	} {
		b := &fakeBackend{}
		c := controller{b: b, buttons: 1, keys: []core.Event{{Kind: core.Key, A: 0xffe1, B: 0x2a}}, releaseAt: time.Now().Add(time.Minute)}
		if e := c.perform(context.Background(), a); e != core.InvalidArgs {
			t.Fatalf("%s: %v", a.Kind, e)
		}
		if c.holding() {
			t.Fatalf("%s failed to clean up", a.Kind)
		}
		if len(b.events) != 2 || b.events[0].C != 0 || b.events[1].C != 0 {
			t.Fatalf("sent conflicting input: %+v", b.events)
		}
	}
}

func (f *fakeBackend) Info() (core.Info, error) {
	return core.Info{State: "ready", Width: 640, Height: 480, Geometry: f.geometry}, nil
}
func (f *fakeBackend) Snapshot() (*core.Frame, error) { return nil, core.Unsupported }
func (f *fakeBackend) Close()                         {}
func (f *fakeBackend) Send(ctx context.Context, e core.Event) error {
	f.events = append(f.events, e)
	if f.failAt > 0 && len(f.events) == f.failAt {
		return core.NetworkError
	}
	return nil
}

func TestChordReleasesAfterPartialFailure(t *testing.T) {
	b := &fakeBackend{failAt: 2}
	c := controller{b: b}
	if e := c.perform(context.Background(), core.Action{Kind: "key", Chord: "CTRL+SHIFT+A"}); e != core.NetworkError {
		t.Fatal(e)
	}
	if len(b.events) != 4 || b.events[2].A != 0xffe1 || b.events[2].C != 0 || b.events[3].A != 0xffe3 || b.events[3].C != 0 {
		t.Fatalf("keys not released: %+v", b.events)
	}
}

func TestInvalidCoordinatesAndStaleGeometrySendNothing(t *testing.T) {
	for _, a := range []core.Action{{Kind: "click", X: 640, Y: 0}, {Kind: "move", X: -1, Y: 2}, {Kind: "click", X: 1, Y: 1, Geometry: 3}, {Kind: "drag", X: 0, Y: 0, ToX: 1, ToY: 480, Duration: time.Second}} {
		b := &fakeBackend{geometry: 2}
		c := controller{b: b}
		if c.perform(context.Background(), a) == nil || len(b.events) != 0 {
			t.Fatalf("invalid action sent: %+v", a)
		}
	}
}

func TestCanceledDragReleasesMouse(t *testing.T) {
	b := &fakeBackend{}
	c := controller{b: b}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	e := c.perform(ctx, core.Action{Kind: "drag", X: 2, Y: 3, ToX: 30, ToY: 40, Duration: time.Second})
	if e != core.TimedOut {
		t.Fatal(e)
	}
	if len(b.events) < 3 || b.events[len(b.events)-1].C != 0 {
		t.Fatal("mouse remains held")
	}
}

func TestSafeErrorBoundary(t *testing.T) {
	secret := "password=PRIVATE host=PRIVATE user=PRIVATE"
	for _, e := range []error{errors.New(secret), core.Error(secret)} {
		b, _ := json.Marshal(core.Result{Error: core.Code(e)})
		if strings.Contains(string(b), "PRIVATE") || !strings.Contains(string(b), "OPERATION_FAILED") {
			t.Fatal(string(b))
		}
	}
}
