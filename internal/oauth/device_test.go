package oauth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPollDeviceCompletesAfterPendingAndSlowDown verifies generic device polling timing states.
func TestPollDeviceCompletesAfterPendingAndSlowDown(t *testing.T) {
	attempts := 0
	err := PollDevice(context.Background(), DevicePollConfig{
		Interval:         time.Millisecond,
		SlowDownIncrease: time.Millisecond,
		Timeout:          time.Second,
	}, func(context.Context) (DevicePollStatus, error) {
		attempts++

		if attempts == 1 {
			return DevicePollPending, nil
		}

		if attempts == 2 {
			return DevicePollSlowDown, nil
		}

		return DevicePollComplete, nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, attempts)
}

// TestPollDeviceTimesOut verifies polling respects the configured timeout.
func TestPollDeviceTimesOut(t *testing.T) {
	err := PollDevice(context.Background(), DevicePollConfig{
		Interval: time.Millisecond,
		Timeout:  time.Millisecond,
	}, func(context.Context) (DevicePollStatus, error) {
		return DevicePollPending, nil
	})

	assert.ErrorContains(t, err, "device authorization timed out")
}
