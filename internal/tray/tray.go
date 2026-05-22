// Package tray owns the macOS menu-bar (status bar) item that keeps Proximity
// alive when the window is closed. The window is "close-to-hide"; the only way
// for the user to quit is the Quit entry in this menu.
//
// We talk directly to Cocoa (NSStatusBar / NSMenu) via CGo because Wails v2
// already owns the NSApp delegate and run loop on the main thread; layering a
// second tray library on top (e.g. energye/systray) would steal the delegate
// and crash on launch.
package tray

import "context"

// Hooks lets main.go inject Wails-runtime actions without the tray package
// depending on the App struct directly.
type Hooks struct {
	OnShow func()
	OnQuit func()
}

// Start registers the status-bar item and returns a stop func that removes
// the item on Wails shutdown. Dock visibility is *not* changed here — call
// SetDockVisible to drive it from window-visibility events. iconBytes must
// be a PNG decodable by NSImage.
func Start(ctx context.Context, iconBytes []byte, hooks Hooks) func() {
	startNative(ctx, iconBytes, hooks)
	return stopNative
}

// SetDockVisible toggles macOS activation policy:
//   - true  -> Regular (Dock icon, app menu, cmd-tab, brought to front)
//   - false -> Accessory (Dock icon hidden; menu bar status item unaffected)
//
// No-op on non-darwin platforms.
func SetDockVisible(visible bool) {
	setDockVisibleNative(visible)
}
