package proxy

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	maxRetryAfter     = 30 * time.Second
	defaultRetryAfter = 1 * time.Second
)

type rateLimitCoordinator struct {
	mu       sync.Mutex
	cooldown map[string]time.Time
}

func newRateLimitCoordinator() *rateLimitCoordinator {
	return &rateLimitCoordinator{
		cooldown: make(map[string]time.Time),
	}
}

func (c *rateLimitCoordinator) waitFor(host string) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()

	until, ok := c.cooldown[host]
	if !ok {
		return 0
	}

	remaining := time.Until(until)
	if remaining <= 0 {
		delete(c.cooldown, host)
		return 0
	}

	return remaining
}

func (c *rateLimitCoordinator) markRetryAfter(host string, wait time.Duration) {
	if wait <= 0 {
		return
	}

	if wait > maxRetryAfter {
		wait = maxRetryAfter
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.cooldown[host] = time.Now().Add(wait)
}

// parseRetryAfter inspects Retry-After first (RFC 7231: delta-seconds or
// HTTP-date), then falls back to X-RateLimit-Reset which AI Gateway emits
// as an epoch-seconds timestamp. Returns defaultRetryAfter when nothing
// parses, so callers always get a usable wait.
func parseRetryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return defaultRetryAfter
	}

	if header := resp.Header.Get("Retry-After"); header != "" {
		if secs, err := strconv.Atoi(header); err == nil {
			if secs > 0 {
				return time.Duration(secs) * time.Second
			}
		} else if when, err := http.ParseTime(header); err == nil {
			if wait := time.Until(when); wait > 0 {
				return wait
			}
		}
	}

	if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
		if wait := parseResetTimestamp(reset); wait > 0 {
			return wait
		}
	}

	return defaultRetryAfter
}

// parseResetTimestamp accepts the three formats AI Gateway / Heimdall could
// plausibly emit: epoch seconds, epoch milliseconds, or RFC 3339 / HTTP-date.
// The seconds-vs-millis distinction uses a magnitude heuristic — any plain
// integer past the year 3000 in seconds is treated as milliseconds, which
// covers epoch-millis cleanly without needing the caller to know the unit.
func parseResetTimestamp(value string) time.Duration {
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		const secondsCutoff = int64(32503680000) // 3000-01-01T00:00:00Z
		var when time.Time
		if n > secondsCutoff {
			when = time.UnixMilli(n)
		} else {
			when = time.Unix(n, 0)
		}
		return time.Until(when)
	}

	if when, err := time.Parse(time.RFC3339, value); err == nil {
		return time.Until(when)
	}

	if when, err := http.ParseTime(value); err == nil {
		return time.Until(when)
	}

	return 0
}
