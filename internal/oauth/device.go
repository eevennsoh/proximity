package oauth

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	defaultDevicePollInterval         = 5 * time.Second
	defaultDevicePollSlowDownIncrease = 5 * time.Second
)

// DevicePollStatus describes the result of one device authorization poll.
type DevicePollStatus string

const (
	// DevicePollPending means authorization has not completed yet.
	DevicePollPending DevicePollStatus = "pending"
	// DevicePollSlowDown means future polls should wait longer.
	DevicePollSlowDown DevicePollStatus = "slow_down"
	// DevicePollComplete means authorization completed successfully.
	DevicePollComplete DevicePollStatus = "complete"
	// DevicePollFailed means authorization reached a terminal failure.
	DevicePollFailed DevicePollStatus = "failed"
)

// DevicePollConfig configures generic device authorization polling.
type DevicePollConfig struct {
	Interval         time.Duration
	SlowDownIncrease time.Duration
	Timeout          time.Duration
}

// DevicePollFunc performs one provider-specific device authorization poll.
type DevicePollFunc func(ctx context.Context) (DevicePollStatus, error)

// PollDevice polls a provider-specific device authorization endpoint until completion.
func PollDevice(ctx context.Context, config DevicePollConfig, poll DevicePollFunc) error {
	config = normalizeDevicePollConfig(config)
	pollCtx, cancel := contextWithDevicePollTimeout(ctx, config.Timeout)
	delay := config.Interval

	defer cancel()

	for {
		status, err := poll(pollCtx)
		if err != nil {
			return err
		}

		switch status {
		case DevicePollComplete:
			return nil
		case DevicePollFailed:
			return fmt.Errorf("device authorization failed")
		case DevicePollSlowDown:
			delay += config.SlowDownIncrease
		case DevicePollPending:
		default:
			return fmt.Errorf("unknown device authorization status %q", status)
		}

		if err := waitForDevicePollDelay(pollCtx, delay); err != nil {
			return err
		}
	}
}

// normalizeDevicePollConfig fills in device polling timing defaults.
func normalizeDevicePollConfig(config DevicePollConfig) DevicePollConfig {
	if config.Interval <= 0 {
		config.Interval = defaultDevicePollInterval
	}

	if config.SlowDownIncrease <= 0 {
		config.SlowDownIncrease = defaultDevicePollSlowDownIncrease
	}

	return config
}

// contextWithDevicePollTimeout returns a context bound by the optional polling timeout.
func contextWithDevicePollTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}

	return context.WithTimeout(ctx, timeout)
}

// waitForDevicePollDelay waits between device authorization polls.
func waitForDevicePollDelay(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("device authorization timed out: %w", ctx.Err())
		}

		return ctx.Err()
	}
}
