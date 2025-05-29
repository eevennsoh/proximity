package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bitbucket.org/atlassian-developers/mini-proxy/internal/config"
	"bitbucket.org/atlassian-developers/mini-proxy/internal/proxy"

	"github.com/alexflint/go-arg"
)

var Config string

type Args struct {
	Port int `arg:"--port" default:"3001"`
}

// awaitStopSignal awaits termination signals and shutdown gracefully by cancelling the context
func awaitStopSignal(cancelFunc context.CancelFunc) {
	defer cancelFunc()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-signalChan

	log.Println("Signal received: ", sig)
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	go awaitStopSignal(cancel)

	var args Args
	arg.MustParse(&args)

	fmt.Println(Config)

	cfg, err := config.ReadConfig(Config)
	if err != nil {
		log.Fatal(err)
	}

	p := proxy.New(cfg, proxy.Options{
		Port: args.Port,
	})

	go p.RunServer(ctx)

	// wait idle until termination
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := p.Shutdown(shutdownCtx); err != nil {
		log.Fatal(err)
	}
}
