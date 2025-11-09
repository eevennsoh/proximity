package proxy

import (
	"context"
	"log"

	"bitbucket.org/atlassian-developers/mini-proxy/internal/config"
	"bitbucket.org/atlassian-developers/mini-proxy/internal/settings"
)

type Options struct {
	Port     int
	TestMode bool
	Logger   *log.Logger

	*config.Config
	Settings          *settings.Struct
	TemplateVariables map[string]any
}

type Interface interface {
	RunServer(ctx context.Context)
	Shutdown(ctx context.Context) error
}
