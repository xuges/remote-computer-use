package backend

import (
	"context"
	"github.com/xuges/remote-computer-use/internal/config"
	"github.com/xuges/remote-computer-use/internal/core"
)

type Backend interface {
	Info() (core.Info, error)
	Snapshot() (*core.Frame, error)
	Send(context.Context, core.Event) error
	Close()
}

// Open is replaced by the native loader in the protocol implementation stage.
func Open(c config.Private) (Backend, error) { return nil, core.MissingBackend }
