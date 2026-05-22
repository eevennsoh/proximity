package proxy

import (
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"time"
)

const (
	maxRetryAttempts  = 3
	maxBackoff        = 30 * time.Second
	upstreamRateLimit = "UPSTREAM"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func (s *server) retryTransport() http.RoundTripper {
	base := http.DefaultTransport

	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		host := req.URL.Host

		if err := s.waitForCooldown(req, host); err != nil {
			return nil, err
		}

		resp, err := base.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		for attempt := 1; attempt <= maxRetryAttempts && shouldRetry(resp); attempt++ {
			limitType := resp.Header.Get("X-RateLimit-Type")
			wait := nextWait(resp, limitType, attempt)

			s.rateLimiter.markRetryAfter(host, wait)
			s.Logger.Printf("rate-limited by %s (status=%d type=%q), retry %d/%d after %s",
				host, resp.StatusCode, limitType, attempt, maxRetryAttempts, wait)

			drainAndClose(resp)

			if req.GetBody == nil {
				return nil, &nonReplayableError{host: host}
			}

			newBody, getBodyErr := req.GetBody()
			if getBodyErr != nil {
				return nil, getBodyErr
			}
			req.Body = newBody

			if err := sleep(req, wait); err != nil {
				return nil, err
			}

			resp, err = base.RoundTrip(req)
			if err != nil {
				return nil, err
			}
		}

		return resp, nil
	})
}

func (s *server) waitForCooldown(req *http.Request, host string) error {
	wait := s.rateLimiter.waitFor(host)
	if wait <= 0 {
		return nil
	}

	wait = capWait(jitter(wait))
	s.Logger.Printf("waiting %s for %s rate-limit window to clear", wait, host)
	return sleep(req, wait)
}

func shouldRetry(resp *http.Response) bool {
	return resp != nil && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable)
}

// nextWait picks the larger of the server's stated Retry-After and a
// local exponential backoff. UPSTREAM 429s come from variable vendor
// capacity, so backoff helps; MODEL.USE_CASE / USER 429s are quota
// boundaries where extra wait beyond Retry-After is wasted.
//
// The final value is jittered upward so that N concurrent retries to the
// same host wake on a spread, not in lockstep. Jitter is always additive
// so we never wait less than what the gateway asked for.
func nextWait(resp *http.Response, limitType string, attempt int) time.Duration {
	stated := parseRetryAfter(resp)
	base := stated

	if limitType == upstreamRateLimit {
		backoff := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
		if backoff > base {
			base = backoff
		}
	}

	return capWait(jitter(base))
}

// jitter adds a uniform-random delay of [0, max(d/4, 100ms)] on top of d.
// Always additive: a server-stated Retry-After is a floor, never a ceiling.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}

	bound := d / 4
	if bound < 100*time.Millisecond {
		bound = 100 * time.Millisecond
	}

	return d + time.Duration(rand.Int64N(int64(bound)))
}

func capWait(d time.Duration) time.Duration {
	if d > maxBackoff {
		return maxBackoff
	}
	if d <= 0 {
		return defaultRetryAfter
	}
	return d
}

func sleep(req *http.Request, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-req.Context().Done():
		return req.Context().Err()
	}
}

func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

type nonReplayableError struct {
	host string
}

func (e *nonReplayableError) Error() string {
	return "proxy: cannot retry rate-limited request to " + e.host + ": request has no GetBody"
}
