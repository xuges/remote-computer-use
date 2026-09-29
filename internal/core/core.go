package core

import "time"

type Error string

func (e Error) Error() string { return string(e) }

const (
	InvalidConfig   Error = "CONFIG_INVALID"
	InvalidArgs     Error = "INVALID_ARGUMENT"
	NotConnected    Error = "NOT_CONNECTED"
	NotReady        Error = "NOT_READY"
	MissingBackend  Error = "BACKEND_UNAVAILABLE"
	ConnectFailed   Error = "CONNECT_FAILED"
	AuthFailed      Error = "AUTH_FAILED"
	TLSFailed       Error = "TLS_VERIFY_FAILED"
	NetworkError    Error = "CONNECTION_LOST"
	TimedOut        Error = "TIMEOUT"
	Canceled        Error = "CANCELED"
	Unsupported     Error = "UNSUPPORTED"
	GeometryChanged Error = "GEOMETRY_CHANGED"
	IOError         Error = "LOCAL_IO_FAILED"
	InternalError   Error = "OPERATION_FAILED"
)

// Code is an allowlist, never a rendering of a third-party error.
func Code(err error) Error {
	if e, ok := err.(Error); ok {
		switch e {
		case InvalidConfig, InvalidArgs, NotConnected, NotReady, MissingBackend, ConnectFailed, AuthFailed, TLSFailed, NetworkError, TimedOut, Canceled, Unsupported, GeometryChanged, IOError, InternalError:
			return e
		}
	}
	return InternalError
}

type Info struct {
	Name         string   `json:"name"`
	Protocol     string   `json:"protocol,omitempty"`
	State        string   `json:"state"`
	Width        int      `json:"width,omitempty"`
	Height       int      `json:"height,omitempty"`
	Geometry     uint64   `json:"geometry_version,omitempty"`
	Sequence     uint64   `json:"frame_sequence,omitempty"`
	Busy         bool     `json:"busy"`
	Capabilities []string `json:"capabilities,omitempty"`
	Error        Error    `json:"error,omitempty"`
}

type Frame struct {
	Width, Height      int
	Geometry, Sequence uint64
	Pixels             []byte // tightly packed RGBA, owned by Go
}

type Event struct{ Kind, A, B, C, D int }

const (
	Pointer = 1
	Key     = 2
	Unicode = 3
	Wheel   = 4
)

type Action struct {
	Kind     string        `json:"kind"`
	X        int           `json:"x"`
	Y        int           `json:"y"`
	ToX      int           `json:"to_x"`
	ToY      int           `json:"to_y"`
	DX       int           `json:"dx"`
	DY       int           `json:"dy"`
	Button   string        `json:"button,omitempty"`
	Count    int           `json:"count,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`
	Chord    string        `json:"chord,omitempty"`
	Text     string        `json:"text,omitempty"`
	Method   string        `json:"method,omitempty"`
	Geometry uint64        `json:"geometry_version,omitempty"`
}

type Request struct {
	Operation string        `json:"operation"`
	Timeout   time.Duration `json:"timeout"`
	Action    Action        `json:"action"`
}

type Result struct {
	OK       bool   `json:"ok"`
	Error    Error  `json:"error,omitempty"`
	Info     *Info  `json:"connection,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Geometry uint64 `json:"geometry_version,omitempty"`
	Sequence uint64 `json:"frame_sequence,omitempty"`
	Output   string `json:"output,omitempty"`
	PNG      []byte `json:"png,omitempty"`
}
