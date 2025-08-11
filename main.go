// package main

// import (
// 	"context"
// 	"encoding/base64"
// 	"log"
// 	"os"
// 	"os/signal"
// 	"syscall"
// 	"time"

// 	"bitbucket.org/atlassian-developers/mini-proxy/internal/config"
// 	"bitbucket.org/atlassian-developers/mini-proxy/internal/proxy"

// 	"github.com/alexflint/go-arg"
// )

// var Config string

// type Args struct {
// 	Port     int  `arg:"--port" default:"3001"`
// 	TestMode bool `arg:"--test-mode" default:"false"`
// }

// // awaitStopSignal awaits termination signals and shutdown gracefully by cancelling the context
// func awaitStopSignal(cancelFunc context.CancelFunc) {
// 	defer cancelFunc()

// 	signalChan := make(chan os.Signal, 1)
// 	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
// 	sig := <-signalChan

// 	log.Println("Signal received: ", sig)
// }

// func main() {
// 	ctx, cancel := context.WithCancel(context.Background())
// 	go awaitStopSignal(cancel)

// 	var args Args
// 	arg.MustParse(&args)

// 	cfgB64 := Config
// 	if cfgB64 == "" {
// 		if v := os.Getenv("MINI_PROXY_CONFIG_B64"); v != "" {
// 			cfgB64 = v
// 		} else if p := os.Getenv("MINI_PROXY_CONFIG_PATH"); p != "" {
// 			data, err := os.ReadFile(p)
// 			if err != nil {
// 				log.Fatal(err)
// 			}
// 			cfgB64 = base64.StdEncoding.EncodeToString(data)
// 		} else if data, err := os.ReadFile("config.yaml"); err == nil {
// 			cfgB64 = base64.StdEncoding.EncodeToString(data)
// 		} else {
// 			log.Fatal("no config provided via ldflags, MINI_PROXY_CONFIG_B64, embedded config, MINI_PROXY_CONFIG_PATH, or config.yaml")
// 		}
// 	}

// 	cfg, err := config.ReadConfig(cfgB64)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	p := proxy.New(cfg, proxy.Options{
// 		Port:     args.Port,
// 		TestMode: args.TestMode,
// 	})

// 	go p.RunServer(ctx)

// 	// wait idle until termination
// 	<-ctx.Done()

// 	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
// 	defer cancel()

// 	if err := p.Shutdown(shutdownCtx); err != nil {
// 		log.Fatal(err)
// 	}
// }

package main

import (
	"embed"
	"log"
	"strconv"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"bitbucket.org/atlassian-developers/mini-proxy/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

var Config string
var Port string

func main() {
	port, err := strconv.Atoi(Port)
	if err != nil {
		log.Fatal(err)
	}

	app := app.NewApp(Config, port)

	// Create application with options
	err = wails.Run(&options.App{
		Title:     "Proximity",
		Width:     1024,
		Height:    768,
		Frameless: false,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		// Mac: &mac.Options{
		// 	TitleBar: &mac.TitleBar{
		// 		TitlebarAppearsTransparent: true,
		// 		HideTitle:                  true,
		// 		FullSizeContent:            true,
		// 		UseToolbar:                 false,
		// 		HideToolbarSeparator:       true,
		// 	},
		// },
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.Startup,
		Bind: []any{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
