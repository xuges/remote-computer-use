package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/xuges/remote-computer-use/internal/core"
	"strings"
	"testing"
	"time"
)

type fakeBackend struct {
	events   []core.Event
	failAt   int
	geometry uint64
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
