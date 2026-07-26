package service

import (
	"net/http"
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

func TestUsageResponseTimingPrefersUpstreamReportedFirstByte(t *testing.T) {
	startedAt := time.Unix(100, 0)
	timing := NewUsageResponseTiming(startedAt)

	// Locally we observe 900ms: the hop plus however long the upstream gateway
	// held the stream before releasing it. The gateway reports the 340ms it
	// actually waited on its own upstream, and that is what we record.
	timing.ObserveUpstreamRead(startedAt.Add(900 * time.Millisecond))
	require.Equal(t, 900, *timing.FirstByteMs())

	timing.ObserveUpstreamReportedFirstByte(http.Header{
		http.CanonicalHeaderKey(UpstreamFirstByteMsHeader): []string{"340"},
	})
	require.Equal(t, 340, *timing.FirstByteMs())

	// Total duration stays on our own clock; only first byte is adopted.
	duration, ok := timing.UpstreamDuration()
	require.True(t, ok)
	require.Equal(t, 900*time.Millisecond, duration)
}

func TestUsageResponseTimingIgnoresUnusableUpstreamFirstByte(t *testing.T) {
	startedAt := time.Unix(100, 0)
	timing := NewUsageResponseTiming(startedAt)
	timing.ObserveUpstreamRead(startedAt.Add(50 * time.Millisecond))

	for _, raw := range []string{"", "   ", "abc", "-1"} {
		timing.ObserveUpstreamReportedFirstByte(http.Header{
			http.CanonicalHeaderKey(UpstreamFirstByteMsHeader): []string{raw},
		})
		require.Equal(t, 50, *timing.FirstByteMs(), "raw value %q must be ignored", raw)
	}

	timing.ObserveUpstreamReportedFirstByte(nil)
	require.Equal(t, 50, *timing.FirstByteMs())
}

func TestUsageResponseTimingDropsUpstreamFirstByteOnRetry(t *testing.T) {
	startedAt := time.Unix(100, 0)
	timing := NewUsageResponseTiming(startedAt)

	timing.ObserveUpstreamRead(startedAt.Add(80 * time.Millisecond))
	timing.ObserveUpstreamReportedFirstByte(http.Header{
		http.CanonicalHeaderKey(UpstreamFirstByteMsHeader): []string{"12"},
	})
	require.Equal(t, 12, *timing.FirstByteMs())

	// The retried attempt has its own upstream; the stale value must not leak.
	timing.BeginUpstreamResponse()
	require.Nil(t, timing.FirstByteMs())

	timing.ObserveUpstreamRead(startedAt.Add(200 * time.Millisecond))
	require.Equal(t, 200, *timing.FirstByteMs())
}

func TestUsageResponseTimingIgnoresObservationBeforeStart(t *testing.T) {
	startedAt := time.Unix(100, 0)
	timing := NewUsageResponseTiming(startedAt)

	timing.ObserveUpstreamRead(startedAt.Add(-time.Millisecond))

	require.Nil(t, timing.FirstByteMs())
	_, ok := timing.UpstreamDuration()
	require.False(t, ok)
}
