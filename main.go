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
