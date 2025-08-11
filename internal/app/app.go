package app

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"sync"

	"bitbucket.org/atlassian-developers/mini-proxy/internal/config"
	"bitbucket.org/atlassian-developers/mini-proxy/internal/proxy"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx context.Context
	mu  sync.Mutex
	// cmd     *exec.Cmd
	running bool
	logs    bytes.Buffer

	proxy  proxy.Interface
	config string
}

// NewApp creates a new App application struct
func NewApp(config string) *App {
	return &App{
		config: config,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// StartProxy builds (if needed) and starts the proxy as a subprocess
func (a *App) StartProxy(configPath string, port int) error {
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

	// Create a pipe and a logger that writes to it so we can stream proxy logs to the UI
	pr, pw := io.Pipe()
	logger := log.New(pw, "", log.LstdFlags)

	a.proxy = proxy.New(cfg, proxy.Options{
		Port:     port,
		TestMode: false,
		Logger:   logger,
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
