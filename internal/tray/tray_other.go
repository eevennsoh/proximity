//go:build !darwin

package tray

import "context"

func startNative(_ context.Context, _ []byte, _ Hooks) {}

func stopNative() {}

func setDockVisibleNative(_ bool) {}
