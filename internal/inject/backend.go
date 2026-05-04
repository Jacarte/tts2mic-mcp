package inject

import (
    "context"
    "errors"
)

type Backend interface {
    Inject(ctx context.Context, wav []byte) error
}

func NewBackend(name string) Backend {
    switch name {
    case "chrome-file":
        return &chromeFile{}
    default:
        return &noop{}
    }
}

// default no-op

type noop struct{}

func (n *noop) Inject(ctx context.Context, wav []byte) error {
    return errors.New("unknown backend")
}
