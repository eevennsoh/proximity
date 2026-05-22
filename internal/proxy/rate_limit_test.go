package proxy

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestParseRetryAfter_DeltaSeconds(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "7")

	got := parseRetryAfter(resp)
	want := 7 * time.Second

	if got != want {
		t.Fatalf("parseRetryAfter delta-seconds: got %s, want %s", got, want)
	}
}

func TestParseRetryAfter_HttpDate(t *testing.T) {
	target := time.Now().Add(12 * time.Second).UTC().Truncate(time.Second)

	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", target.Format(http.TimeFormat))

	got := parseRetryAfter(resp)

	if got < 10*time.Second || got > 13*time.Second {
		t.Fatalf("parseRetryAfter http-date: got %s, want ~12s", got)
	}
}

func TestParseRetryAfter_FallsBackToResetEpochSeconds(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("X-RateLimit-Reset", formatEpochSeconds(time.Now().Add(15*time.Second)))

	got := parseRetryAfter(resp)

	if got < 13*time.Second || got > 16*time.Second {
		t.Fatalf("parseRetryAfter X-RateLimit-Reset (seconds): got %s, want ~15s", got)
	}
}

func TestParseRetryAfter_ResetEpochMillis(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("X-RateLimit-Reset", formatEpochMillis(time.Now().Add(20*time.Second)))

	got := parseRetryAfter(resp)

	if got < 18*time.Second || got > 21*time.Second {
		t.Fatalf("parseRetryAfter X-RateLimit-Reset (millis): got %s, want ~20s", got)
	}
}

func TestParseRetryAfter_ResetRfc3339(t *testing.T) {
	target := time.Now().Add(25 * time.Second).UTC().Truncate(time.Second)

	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("X-RateLimit-Reset", target.Format(time.RFC3339))

	got := parseRetryAfter(resp)

	if got < 22*time.Second || got > 26*time.Second {
		t.Fatalf("parseRetryAfter X-RateLimit-Reset (RFC3339): got %s, want ~25s", got)
	}
}

func TestParseRetryAfter_MissingHeaders(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}

	if got := parseRetryAfter(resp); got != defaultRetryAfter {
		t.Fatalf("parseRetryAfter no headers: got %s, want %s", got, defaultRetryAfter)
	}
}

func TestParseRetryAfter_NilResponse(t *testing.T) {
	if got := parseRetryAfter(nil); got != defaultRetryAfter {
		t.Fatalf("parseRetryAfter nil: got %s, want %s", got, defaultRetryAfter)
	}
}

func TestParseRetryAfter_GarbageHeaders(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "not-a-number")
	resp.Header.Set("X-RateLimit-Reset", "also-not-a-number")

	if got := parseRetryAfter(resp); got != defaultRetryAfter {
		t.Fatalf("parseRetryAfter garbage: got %s, want %s", got, defaultRetryAfter)
	}
}

func formatEpochSeconds(t time.Time) string {
	return strconv.FormatInt(t.Unix(), 10)
}

func formatEpochMillis(t time.Time) string {
	return strconv.FormatInt(t.UnixMilli(), 10)
}
