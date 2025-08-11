package proxy

import (
	"context"
	"log"
)

type Options struct {
	Port     int
	TestMode bool
	Logger   *log.Logger
}

type Interface interface {
	RunServer(ctx context.Context)
	Shutdown(ctx context.Context) error
}
