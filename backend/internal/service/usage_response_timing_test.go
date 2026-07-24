package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUsageResponseTimingResetsInternalUpstreamAttempt(t *testing.T) {
	startedAt := time.Unix(100, 0)
	timing := NewUsageResponseTiming(startedAt)

	timing.ObserveUpstreamRead(startedAt.Add(10 * time.Millisecond))
	require.NotNil(t, timing.FirstByteMs())

	timing.BeginUpstreamResponse()
	require.Nil(t, timing.FirstByteMs())
	_, ok := timing.UpstreamDuration()
	require.False(t, ok)

	timing.ObserveUpstreamRead(startedAt.Add(30 * time.Millisecond))
	require.Equal(t, 30, *timing.FirstByteMs())
	duration, ok := timing.UpstreamDuration()
	require.True(t, ok)
	require.Equal(t, 30*time.Millisecond, duration)
}

func TestUsageResponseTimingIgnoresObservationBeforeStart(t *testing.T) {
	startedAt := time.Unix(100, 0)
	timing := NewUsageResponseTiming(startedAt)

	timing.ObserveUpstreamRead(startedAt.Add(-time.Millisecond))

	require.Nil(t, timing.FirstByteMs())
	_, ok := timing.UpstreamDuration()
	require.False(t, ok)
}
