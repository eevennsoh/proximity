package app

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	"bitbucket.org/atlassian-developers/proximity/internal/config"
	"bitbucket.org/atlassian-developers/proximity/internal/proxy"
	"bitbucket.org/atlassian-developers/proximity/internal/settings"
	"bitbucket.org/atlassian-developers/proximity/internal/update"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx     context.Context
	mu      sync.Mutex
	running bool
	logs    bytes.Buffer

	proxy      proxy.Interface
	pipeWriter io.WriteCloser
	port       int

	configPath string
	config     *config.Config

	settingsPath string
	settings     *settings.Struct

	version   string
	changelog string
}

// NewApp creates a new App application struct
func NewApp(configPath string, port int, settingsPath, version, changelog string) *App {
	return &App{
		configPath:   configPath,
		port:         port,
		settingsPath: settingsPath,
		version:      version,
		changelog:    changelog,
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

	a.config, err = config.ReadConfig(a.configPath)
	if err != nil {
		log.Fatal(err)
	}

	if a.settings.AutoStartProxy {
		if err := a.StartProxy(); err != nil {
			log.Printf("Failed to auto-start proxy: %v", err)
		}
	}

	if err := update.NofifyIfNewVersionExists(a.version); err != nil {
		log.Println(err)
	}
}

func (a *App) StartProxy() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return errors.New("proxy already running")
	}

	a.logs.Reset()

	// Create a pipe and a logger that writes to it so we can stream proxy logs to the UI
	pr, pw := io.Pipe()
	a.pipeWriter = pw
	logger := log.New(pw, "", log.LstdFlags)

	a.proxy = proxy.New(proxy.Options{
		Port:     a.port,
		TestMode: false,
		Logger:   logger,
		Config:   a.config,
		Vars:     a.settings.Vars,
		Version:  a.version,
	})

	a.running = true
	go a.pipeLogs(pr)

	go a.proxy.RunServer(a.ctx)

	a.logSettings(logger)

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

	// Close the pipe writer to unblock the pipeLogs goroutine
	if a.pipeWriter != nil {
		a.pipeWriter.Close()
		a.pipeWriter = nil
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

// EndpointsResponse is the structure returned to the frontend
type EndpointsResponse struct {
	BaseEndpoint string            `json:"baseEndpoint"`
	UriGroups    []config.UriGroup `json:"uriGroups"`
}

// GetEndpoints returns the configured base endpoint and supported URI mappings.
// Hidden URI groups are filtered out from the response.
func (a *App) GetEndpoints() (*EndpointsResponse, error) {
	visibleGroups := make([]config.UriGroup, 0, len(a.config.UriGroups))

	for _, group := range a.config.UriGroups {
		if !group.Hidden {
			visibleGroups = append(visibleGroups, group)
		}
	}

	return &EndpointsResponse{
		BaseEndpoint: a.config.BaseEndpoint,
		UriGroups:    visibleGroups,
	}, nil
}

func (a *App) GetPort() int {
	return a.port
}

// GetChangelog returns the changelog content and version for display
func (a *App) GetChangelog() map[string]string {
	return map[string]string{
		"version":   a.version,
		"changelog": a.changelog,
	}
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

func (a *App) logSettings(logger *log.Logger) {
	logLineParts := []string{}

	for key, value := range a.settings.Vars {
		logLineParts = append(logLineParts, fmt.Sprintf("%s=%s", key, value))
	}

	logger.Printf("loading variables %v", strings.Join(logLineParts, " "))
}
