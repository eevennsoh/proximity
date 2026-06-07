package server

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bitbucket.org/atlassian-developers/proximity/internal/config"
	"bitbucket.org/atlassian-developers/proximity/internal/proxy"
	proximitytemplate "bitbucket.org/atlassian-developers/proximity/internal/template"
)

type Options struct {
	Config          *config.Config
	Port            int
	Vars            map[string]any
	TemplateOptions []proximitytemplate.Option
}

// RunServer starts the proxy with the default server options.
func RunServer(cfg *config.Config, port int, vars map[string]any) error {
	return RunServerWithOptions(Options{
		Config: cfg,
		Port:   port,
		Vars:   vars,
	})
}

// RunServerWithOptions starts the proxy with command-specific collaborators.
func RunServerWithOptions(options Options) error {
	logger := log.Default()

	ctx, cancel := context.WithCancel(context.Background())
	go awaitStopSignal(cancel, logger)

	proxyOptions := proxy.Options{
		Port:            options.Port,
		Logger:          logger,
		Config:          options.Config,
		Vars:            options.Vars,
		TemplateOptions: options.TemplateOptions,
	}

	p := proxy.New(proxyOptions)

	go p.RunServer(ctx)

	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := p.Shutdown(shutdownCtx); err != nil {
		return err
	}

	logger.Println("successfully shut down the proxy")
	return nil
}

func awaitStopSignal(cancelFunc context.CancelFunc, logger *log.Logger) {
	defer cancelFunc()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-signalChan

	logger.Print("signal received: ", sig)
}
