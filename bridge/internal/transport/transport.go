package transport

import (
	"context"
	"errors"
)

type AdapterTransport interface {
	Name() string
	Connect(ctx context.Context) error
	Execute(ctx context.Context, command string, args map[string]any) (map[string]any, error)
	Close() error
}

type LoopbackTransport struct{}

func (l *LoopbackTransport) Name() string { return "loopback" }

func (l *LoopbackTransport) Connect(ctx context.Context) error { return nil }

func (l *LoopbackTransport) Execute(ctx context.Context, command string, args map[string]any) (map[string]any, error) {
	switch command {
	case "ping":
		return map[string]any{"result": "pong"}, nil
	case "echo":
		return map[string]any{"result": "echo", "echo": args}, nil
	default:
		return nil, errors.New("unsupported command")
	}
}

func (l *LoopbackTransport) Close() error { return nil }
