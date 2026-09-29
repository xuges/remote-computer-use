package backend

import (
	"context"
	"github.com/xuges/remote-computer-use/internal/core"
)

type Backend interface {
	Info() (core.Info, error)
	Snapshot() (*core.Frame, error)
	Send(context.Context, core.Event) error
	Close()
}
