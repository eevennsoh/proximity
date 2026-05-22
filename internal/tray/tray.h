#ifndef PROXIMITY_TRAY_H
#define PROXIMITY_TRAY_H

// Called from Go. Schedules tray setup on the main queue so AppKit assertions
// (NSStatusBar must be touched on the main thread) hold even though we get
// invoked from Wails' OnStartup goroutine.
void tray_start(const char *iconBytes, int iconLen);

// Called from Go on shutdown. Removes the status item.
void tray_stop(void);

// Toggles macOS activation policy on the main queue: visible=1 -> Regular
// (Dock icon + app menu + cmd-tab + window brought to front); visible=0 ->
// Accessory (no Dock icon; menu bar status item still active).
void tray_set_dock_visible(int visible);

// Implemented in Go via //export. Called from the Obj-C menu targets.
extern void tray_on_show(void);
extern void tray_on_quit(void);

#endif
