package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"bitbucket.org/atlassian-developers/mini-proxy/internal/config"
	"bitbucket.org/atlassian-developers/mini-proxy/internal/proxy"
	"bitbucket.org/atlassian-developers/mini-proxy/internal/settings"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"gopkg.in/yaml.v3"
)

// App struct
type App struct {
	ctx     context.Context
	mu      sync.Mutex
	running bool
	logs    bytes.Buffer

	proxy             proxy.Interface
	config            string
	templateVariables string
	port              int

	settingsPath string
	settings     *settings.Struct
}

// NewApp creates a new App application struct
func NewApp(config, templateVariables string, port int, settingsPath string) *App {
	return &App{
		config:            config,
		templateVariables: templateVariables,
		port:              port,
		settingsPath:      settingsPath,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	var err error

	// Check if auto-start is enabled
	a.settings, err = settings.Read(a.settingsPath)
	if err != nil {
		log.Printf("Failed to read settings: %v", err)
		return
	}

	if a.settings.AutoStartProxy {
		log.Println("Auto-starting proxy")

		if err := a.StartProxy(); err != nil {
			log.Printf("Failed to auto-start proxy: %v", err)
		}
	}
}

func (a *App) StartProxy() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return errors.New("proxy already running")
	}

	a.logs.Reset()

	cfg, err := config.ReadConfig(a.config)
	if err != nil {
		log.Fatal(err)
	}

	templateVariables, err := readTemplateVariables(a.templateVariables)
	if err != nil {
		log.Fatal(err)
	}

	// Create a pipe and a logger that writes to it so we can stream proxy logs to the UI
	pr, pw := io.Pipe()
	logger := log.New(pw, "", log.LstdFlags)

	a.proxy = proxy.New(proxy.Options{
		Port:              a.port,
		TestMode:          false,
		Logger:            logger,
		Config:            cfg,
		Settings:          a.settings,
		TemplateVariables: templateVariables,
	})

	a.running = true
	go a.pipeLogs(pr)

	go a.proxy.RunServer(a.ctx)

	wruntime.EventsEmit(a.ctx, "proxy:status", "running")
	return nil
}

// StopProxy attempts a graceful shutdown, then force kills if needed
func (a *App) StopProxy() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.running {
		return nil
	}

	if err := a.proxy.Shutdown(a.ctx); err != nil {
		return err
	}

	a.running = false
	wruntime.EventsEmit(a.ctx, "proxy:status", "stopped")
	return nil
}

// IsRunning returns whether the proxy is currently running
func (a *App) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

// GetLogs returns accumulated logs as a string
func (a *App) GetLogs() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.logs.String()
}

// ClearLogs clears the in-memory log buffer
func (a *App) ClearLogs() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.logs.Reset()
	wruntime.EventsEmit(a.ctx, "proxy:log:cleared")
}

// Endpoints structures returned to the frontend
type Endpoint struct {
	In  string `json:"in"`
	Out string `json:"out"`
}

type EndpointsResponse struct {
	BaseEndpoint string     `json:"baseEndpoint"`
	Endpoints    []Endpoint `json:"endpoints"`
}

// GetEndpoints returns the configured base endpoint and supported URI mappings.
// It prefers the embedded/base64 config if available; otherwise falls back to reading config.yaml from disk.
func (a *App) GetEndpoints() (*EndpointsResponse, error) {
	var cfg *config.Config
	var err error

	if strings.TrimSpace(a.config) != "" {
		cfg, err = config.ReadConfig(a.config)
		if err != nil {
			return nil, err
		}
	} else {
		data, readErr := os.ReadFile("config.yaml")
		if readErr != nil {
			return nil, readErr
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		cfg, err = config.ReadConfig(b64)
		if err != nil {
			return nil, err
		}
	}

	eps := make([]Endpoint, 0, len(cfg.SupportedUris))
	for _, um := range cfg.SupportedUris {
		eps = append(eps, Endpoint{In: um.In, Out: um.Out})
	}

	return &EndpointsResponse{
		BaseEndpoint: cfg.BaseEndpoint,
		Endpoints:    eps,
	}, nil
}

func (a *App) GetPort() int {
	return a.port
}

func (a *App) pipeLogs(r io.Reader) {
	scanner := bufio.NewScanner(r)
	// increase buffer size for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		a.mu.Lock()
		a.logs.WriteString(line + "\n")
		a.mu.Unlock()
		wruntime.EventsEmit(a.ctx, "proxy:log", line)
	}
}

func readTemplateVariables(templateVariableData string) (map[string]any, error) {
	decodedConfig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(templateVariableData))
	if err != nil {
		return nil, err
	}

	var templateVariables map[string]any

	if err := yaml.Unmarshal([]byte(decodedConfig), &templateVariables); err != nil {
		return nil, err
	}

	return templateVariables, nil
}
