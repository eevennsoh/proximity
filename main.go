package main

import (
	"embed"
	"log"
	"strconv"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"bitbucket.org/atlassian-developers/mini-proxy/internal/app"
)

var (
	//go:embed all:frontend/dist
	assets embed.FS

	Port              string
	Config            string
	SettingsPath      string
	TemplateVariables string
)

func main() {
	port, err := strconv.Atoi(Port)
	if err != nil {
		log.Fatal(err)
	}

	app := app.NewApp(
		Config,
		TemplateVariables,
		port,
		SettingsPath,
	)

	// Create application with options
	err = wails.Run(&options.App{
		Title:           "Proximity",
		Width:           1024,
		Height:          768,
		CSSDragProperty: "--wails-draggable",
		CSSDragValue:    "drag",
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                  true,
				FullSizeContent:            true,
				UseToolbar:                 false,
				HideToolbarSeparator:       true,
			},
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			Appearance:           mac.NSAppearanceNameDarkAqua,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 0},
		OnStartup:        app.Startup,
		Bind: []any{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
