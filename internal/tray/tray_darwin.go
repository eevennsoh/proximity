//go:build darwin

package tray

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa

#include <stdlib.h>
#include "tray.h"
*/
import "C"

import (
	"context"
	"sync"
	"unsafe"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	hooksMu    sync.Mutex
	currHooks  Hooks
	currCtx    context.Context
)

func startNative(ctx context.Context, iconBytes []byte, hooks Hooks) {
	hooksMu.Lock()
	currHooks = hooks
	currCtx = ctx
	hooksMu.Unlock()

	if len(iconBytes) == 0 {
		C.tray_start(nil, 0)
		return
	}
	C.tray_start((*C.char)(unsafe.Pointer(&iconBytes[0])), C.int(len(iconBytes)))
}

func stopNative() {
	C.tray_stop()
}

func setDockVisibleNative(visible bool) {
	v := C.int(0)
	if visible {
		v = 1
	}
	C.tray_set_dock_visible(v)
}

//export tray_on_show
func tray_on_show() {
	hooksMu.Lock()
	h, ctx := currHooks, currCtx
	hooksMu.Unlock()

	if h.OnShow != nil {
		h.OnShow()
		return
	}
	if ctx != nil {
		wruntime.WindowShow(ctx)
		wruntime.WindowUnminimise(ctx)
	}
}

//export tray_on_quit
func tray_on_quit() {
	hooksMu.Lock()
	h, ctx := currHooks, currCtx
	hooksMu.Unlock()

	if h.OnQuit != nil {
		h.OnQuit()
		return
	}
	if ctx != nil {
		wruntime.Quit(ctx)
	}
}
