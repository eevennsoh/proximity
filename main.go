package main

import (
	"context"
	"embed"
	_ "embed"
	"log"
	"strconv"
	"sync/atomic"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"bitbucket.org/atlassian-developers/proximity/internal/app"
	"bitbucket.org/atlassian-developers/proximity/internal/tray"
)

const (
	name = "Proximity"
)

var (
	//go:embed all:frontend/dist
	assets embed.FS

	//go:embed CHANGELOG.md
	changelog string

	//go:embed build/appicon.png
	trayIcon []byte

	Port         string
	Config       string
	SettingsPath string
	Version      string
	HelpUrl      string
)

func main() {
	port, err := strconv.Atoi(Port)
	if err != nil {
		log.Fatal(err)
	}

	application := app.NewApp(
		Config,
		port,
		SettingsPath,
		Version,
		changelog,
		HelpUrl,
	)

	// Create application menu
	appMenu := menu.NewMenu()
	appMenu.Append(menu.AppMenu())
	appMenu.Append(menu.EditMenu())
	appMenu.Append(menu.WindowMenu())

	var stopTray func()
	// Distinguishes "user clicked the window's close button" (hide) from
	// "user picked Quit in the tray" (actually exit). Set by the tray's
	// OnQuit hook, read by OnBeforeClose to let the close go through.
	var quitRequested atomic.Bool

	// Create application with options
	err = wails.Run(&options.App{
		Title:                    name,
		Width:                    1024,
		Height:                   768,
		CSSDragProperty:          "--wails-draggable",
		CSSDragValue:             "drag",
		Menu:                     appMenu,
		EnableDefaultContextMenu: false,
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
		OnStartup: func(ctx context.Context) {
			application.Startup(ctx)
			stopTray = tray.Start(ctx, trayIcon, tray.Hooks{
				OnShow: func() {
					// Standard menu-bar-app pattern: showing the window
					// brings the Dock icon back and activates the app.
					tray.SetDockVisible(true)
					wruntime.WindowShow(ctx)
					wruntime.WindowUnminimise(ctx)
				},
				OnQuit: func() {
					// Set BEFORE calling Quit. Quit triggers a window close,
					// which runs OnBeforeClose; we need the flag visible
					// there so the close isn't vetoed.
					quitRequested.Store(true)
					wruntime.Quit(ctx)
				},
			})
		},
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			// Tray Quit set the flag — let the close (and the app) exit.
			if quitRequested.Load() {
				return false
			}
			// Otherwise this is a window-close-button press: hide the
			// window AND drop the Dock icon, leaving only the tray.
			wruntime.WindowHide(ctx)
			tray.SetDockVisible(false)
			return true
		},
		OnShutdown: func(ctx context.Context) {
			if stopTray != nil {
				stopTray()
			}
		},
		Bind: []any{
			application,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
