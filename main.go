package main

import (
	"embed"
	_ "embed"
	"log"
	"strconv"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"bitbucket.org/atlassian-developers/proximity/internal/app"
)

const (
	name = "Proximity"
)

var (
	//go:embed all:frontend/dist
	assets embed.FS

	//go:embed CHANGELOG.md
	changelog string

	Port              string
	Config            string
	SettingsPath      string
	TemplateVariables string
	Version           string
)

func main() {
	port, err := strconv.Atoi(Port)
	if err != nil {
		log.Fatal(err)
	}

	application := app.NewApp(
		Config,
		TemplateVariables,
		port,
		SettingsPath,
		Version,
		changelog,
	)

	// Create application menu
	appMenu := menu.NewMenu()
	appMenu.Append(menu.AppMenu())
	appMenu.Append(menu.EditMenu())
	appMenu.Append(menu.WindowMenu())

	// Create application with options
	err = wails.Run(&options.App{
		Title:           name,
		Width:           1024,
		Height:          768,
		CSSDragProperty: "--wails-draggable",
		CSSDragValue:    "drag",
		Menu:            appMenu,
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
			About: &mac.AboutInfo{
				Title:   name,
				Message: "Version " + Version,
			},
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			Appearance:           mac.NSAppearanceNameDarkAqua,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 0},
		OnStartup:        application.Startup,
		Bind: []any{
			application,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
