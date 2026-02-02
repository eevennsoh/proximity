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
)

func RunServer(cfg *config.Config, port int, vars map[string]any) error {
	logger := log.Default()

	ctx, cancel := context.WithCancel(context.Background())
	go awaitStopSignal(cancel, logger)

	options := proxy.Options{
		Port:   port,
		Logger: logger,
		Config: cfg,
		Vars:   vars,
	}

	p := proxy.New(options)

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
