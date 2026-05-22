package proxy

import (
	"testing"
	"time"
)

func TestJitter_AlwaysAdditive(t *testing.T) {
	base := 4 * time.Second
	maxExtra := base / 4

	for i := 0; i < 1000; i++ {
		got := jitter(base)
		if got < base {
			t.Fatalf("jitter must never reduce wait: base=%s got=%s", base, got)
		}
		if got > base+maxExtra {
			t.Fatalf("jitter exceeded bound: base=%s max=%s got=%s", base, base+maxExtra, got)
		}
	}
}

func TestJitter_MinimumBound(t *testing.T) {
	// Tiny base (10ms) should still get up to 100ms of jitter so concurrent
	// retries decorrelate even for short waits.
	base := 10 * time.Millisecond
	floor := base
	ceiling := base + 100*time.Millisecond

	for i := 0; i < 200; i++ {
		got := jitter(base)
		if got < floor || got > ceiling {
			t.Fatalf("jitter on small base outside [%s, %s]: got %s", floor, ceiling, got)
		}
	}
}

func TestJitter_ProducesSpread(t *testing.T) {
	base := 2 * time.Second
	seen := make(map[time.Duration]struct{})

	for i := 0; i < 100; i++ {
		seen[jitter(base)] = struct{}{}
	}

	// With ~500ms of jitter range at nanosecond resolution, 100 draws
	// should produce many distinct values. <5 means we've effectively
	// regressed to a constant.
	if len(seen) < 5 {
		t.Fatalf("jitter produced no spread: %d distinct values in 100 draws", len(seen))
	}
}

func TestJitter_ZeroOrNegativeIsPassthrough(t *testing.T) {
	if got := jitter(0); got != 0 {
		t.Fatalf("jitter(0): want 0, got %s", got)
	}
	if got := jitter(-1 * time.Second); got != -1*time.Second {
		t.Fatalf("jitter(negative): want passthrough, got %s", got)
	}
}
