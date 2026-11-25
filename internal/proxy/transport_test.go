package proxy

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestUseCaseCooldownDoesNotBlockOtherUseCases(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{Options: Options{Logger: log.New(io.Discard, "", 0)}, rateLimiter: newRateLimitCoordinator()}
	s.rateLimiter.markRetryAfter(target.Host, "own", 5*time.Second)
	request := func(useCase string) (*http.Response, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Atlassian-UseCaseId", useCase)
		return s.retryTransport().RoundTrip(req)
	}
	resp, err := request("shared")
	if err != nil {
		t.Fatalf("unlimited use case was blocked: %v", err)
	}
	resp.Body.Close()
	if _, err := request("own"); err == nil {
		t.Fatal("rate-limited use case did not wait")
	}
	s.rateLimiter.markRetryAfter(target.Host, "", 5*time.Second)
	if _, err := request("shared"); err == nil {
		t.Fatal("shared gateway cooldown was bypassed")
	}
}

func TestRateLimitResponseScopesCooldown(t *testing.T) {
	for _, test := range []struct {
		name         string
		status       int
		limitType    string
		otherBlocked bool
	}{
		{"model use case", http.StatusTooManyRequests, "MODEL.USE_CASE", false},
		{"user", http.StatusTooManyRequests, "USER", true},
		{"upstream", http.StatusTooManyRequests, "UPSTREAM", true},
		{"unknown", http.StatusTooManyRequests, "", true},
		{"unavailable", http.StatusServiceUnavailable, "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Atlassian-UseCaseId") == "own" {
					w.Header().Set("Retry-After", "1")
					w.Header().Set("X-RateLimit-Type", test.limitType)
					w.WriteHeader(test.status)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()
			s := &server{Options: Options{Logger: log.New(io.Discard, "", 0)}, rateLimiter: newRateLimitCoordinator()}
			request := func(useCase string) (*http.Response, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
				defer cancel()
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstream.URL, strings.NewReader("{}"))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("X-Atlassian-UseCaseId", useCase)
				return s.retryTransport().RoundTrip(req)
			}
			if _, err := request("own"); err == nil {
				t.Fatal("expected limited request to wait")
			}
			resp, err := request("shared")
			if test.otherBlocked {
				if err == nil {
					resp.Body.Close()
					t.Fatal("shared cooldown was bypassed")
				}
			} else {
				if err != nil {
					t.Fatalf("other use case was blocked: %v", err)
				}
				resp.Body.Close()
			}
		})
	}
}

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
