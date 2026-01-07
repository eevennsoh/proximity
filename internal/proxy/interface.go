package proxy

import (
	"context"
	"log"

	"bitbucket.org/atlassian-developers/proximity/internal/config"
	"bitbucket.org/atlassian-developers/proximity/internal/settings"
)

type Options struct {
	Port     int
	TestMode bool
	Version  string

	Logger *log.Logger

	*config.Config
	Settings          *settings.Struct
	TemplateVariables map[string]any
}

type Interface interface {
	RunServer(ctx context.Context)
	Shutdown(ctx context.Context) error
}
