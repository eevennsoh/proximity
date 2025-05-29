package proxy

import (
	"context"
)

type Options struct {
	Port int
}

type Interface interface {
	RunServer(ctx context.Context)
	Shutdown(ctx context.Context) error
}
