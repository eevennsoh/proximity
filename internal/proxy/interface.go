package proxy

import (
	"context"
)

type Options struct {
	Port     int
	TestMode bool
}

type Interface interface {
	RunServer(ctx context.Context)
	Shutdown(ctx context.Context) error
}
