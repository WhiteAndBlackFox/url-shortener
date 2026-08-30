package shutdown_test

import (
	"testing"
	"time"

	"URLShortener/internal/platform/shutdown"

	"github.com/stretchr/testify/require"
)

func TestWaitTimeout_ReturnsTrueWhenWaitFinishesInTime(t *testing.T) {
	done := shutdown.WaitTimeout(time.Second, func() {})
	require.True(t, done)
}

func TestWaitTimeout_ReturnsFalseWhenWaitDoesNotFinishInTime(t *testing.T) {
	block := make(chan struct{}) // never closed: simulates a stuck shutdown step
	done := shutdown.WaitTimeout(10*time.Millisecond, func() { <-block })
	require.False(t, done)
}
