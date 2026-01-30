package proxy

import (
	"context"
	"log"

	"bitbucket.org/atlassian-developers/proximity/internal/config"
)

type Options struct {
	Port     int
	TestMode bool
	Version  string

	Logger *log.Logger

	*config.Config

	// Generic global variables provided to the config for rendering
	Vars map[string]any
}

type Interface interface {
	RunServer(ctx context.Context)
	Shutdown(ctx context.Context) error
}
