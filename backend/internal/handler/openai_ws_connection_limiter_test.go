package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSConnectionLimiterIndependentDimensions(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxIngressConnections = 3
	cfg.Gateway.OpenAIWS.MaxIngressConnectionsPerUser = 2
	cfg.Gateway.OpenAIWS.MaxIngressConnectionsPerAPIKey = 1
	limiter := newOpenAIWSConnectionLimiter(cfg)

	release1, ok, reason := limiter.acquire(1, 10)
	require.True(t, ok)
	require.Empty(t, reason)
	_, ok, reason = limiter.acquire(1, 10)
	require.False(t, ok)
	require.Equal(t, "api_key", reason)

	release2, ok, _ := limiter.acquire(1, 11)
	require.True(t, ok)
	_, ok, reason = limiter.acquire(1, 12)
	require.False(t, ok)
	require.Equal(t, "user", reason)

	release1.Release()
	release1.Release()
	_, ok, _ = limiter.acquire(2, 10)
	require.True(t, ok)
	release2.Release()
}

func BenchmarkOpenAIWSConnectionLimiterAcquireRelease(b *testing.B) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxIngressConnections = 10000
	cfg.Gateway.OpenAIWS.MaxIngressConnectionsPerUser = 64
	cfg.Gateway.OpenAIWS.MaxIngressConnectionsPerAPIKey = 32
	limiter := newOpenAIWSConnectionLimiter(cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		release, ok, reason := limiter.acquire(1, 1)
		if !ok {
			b.Fatalf("unexpected limiter rejection: %s", reason)
		}
		release.Release()
	}
}
