package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCacheOpenAIWSHTTPFallbackRoundTripAndExpiry(t *testing.T) {
	cache, mini := newOpenAIWSHTTPFallbackTestCache(t)
	ctx := context.Background()
	const sessionKey = "authenticated-session"

	required, err := cache.OpenAIWSHTTPFallbackRequired(ctx, sessionKey)
	require.NoError(t, err)
	require.False(t, required)
	require.NoError(t, cache.MarkOpenAIWSHTTPFallback(ctx, sessionKey, 30*time.Second))
	required, err = cache.OpenAIWSHTTPFallbackRequired(ctx, sessionKey)
	require.NoError(t, err)
	require.True(t, required)

	mini.FastForward(30 * time.Second)
	required, err = cache.OpenAIWSHTTPFallbackRequired(ctx, sessionKey)
	require.NoError(t, err)
	require.False(t, required)

	require.NoError(t, cache.MarkOpenAIWSHTTPFallback(ctx, sessionKey, 30*time.Second))
	required, err = cache.OpenAIWSHTTPFallbackRequired(ctx, sessionKey)
	require.NoError(t, err)
	require.True(t, required, "a later independent outage can start a new window")
}

func TestGatewayCacheOpenAIWSHTTPFallbackIsolation(t *testing.T) {
	cache, _ := newOpenAIWSHTTPFallbackTestCache(t)
	ctx := context.Background()
	require.NoError(t, cache.MarkOpenAIWSHTTPFallback(ctx, "session-a", time.Minute))
	require.NoError(t, cache.SetSessionAccountID(ctx, 7, "session-b", 42, time.Minute))

	required, err := cache.OpenAIWSHTTPFallbackRequired(ctx, "session-b")
	require.NoError(t, err)
	require.False(t, required)
	_, err = cache.GetSessionAccountID(ctx, 7, "session-a")
	require.ErrorIs(t, err, redis.Nil, "fallback markers must not create sticky account bindings")
	accountID, err := cache.GetSessionAccountID(ctx, 7, "session-b")
	require.NoError(t, err)
	require.Equal(t, int64(42), accountID)
}

func TestGatewayCacheOpenAIWSHTTPFallbackRepeatedAndConcurrentMarksDoNotExtendTTL(t *testing.T) {
	cache, mini := newOpenAIWSHTTPFallbackTestCache(t)
	ctx := context.Background()
	const sessionKey = "same-session"
	require.NoError(t, cache.MarkOpenAIWSHTTPFallback(ctx, sessionKey, 30*time.Second))
	mini.FastForward(10 * time.Second)
	require.NoError(t, cache.MarkOpenAIWSHTTPFallback(ctx, sessionKey, time.Minute))

	const writers = 16
	errors := make(chan error, writers)
	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- cache.MarkOpenAIWSHTTPFallback(ctx, sessionKey, time.Minute)
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.Equal(t, 20*time.Second, mini.TTL(openAIWSHTTPFallbackPrefix+sessionKey))
	mini.FastForward(20 * time.Second)
	required, err := cache.OpenAIWSHTTPFallbackRequired(ctx, sessionKey)
	require.NoError(t, err)
	require.False(t, required)
}

func TestGatewayCacheOpenAIWSHTTPFallbackRejectsInvalidArguments(t *testing.T) {
	cache, mini := newOpenAIWSHTTPFallbackTestCache(t)
	ctx := context.Background()
	for _, sessionKey := range []string{"", " \t\n"} {
		required, err := cache.OpenAIWSHTTPFallbackRequired(ctx, sessionKey)
		require.Error(t, err)
		require.False(t, required)
		require.Error(t, cache.MarkOpenAIWSHTTPFallback(ctx, sessionKey, time.Second))
	}
	for _, ttl := range []time.Duration{0, -time.Second} {
		require.Error(t, cache.MarkOpenAIWSHTTPFallback(ctx, "session", ttl))
	}
	require.Empty(t, mini.Keys())
}

func TestGatewayCacheOpenAIWSHTTPFallbackPropagatesRedisErrors(t *testing.T) {
	cache, mini := newOpenAIWSHTTPFallbackTestCache(t)
	ctx := context.Background()
	mini.SetError("ERR fallback cache unavailable")
	required, err := cache.OpenAIWSHTTPFallbackRequired(ctx, "session")
	require.ErrorContains(t, err, "fallback cache unavailable")
	require.False(t, required)
	require.ErrorContains(t, cache.MarkOpenAIWSHTTPFallback(ctx, "session", time.Second), "fallback cache unavailable")
}

func TestGatewayCacheOpenAIWSHTTPFallbackPropagatesCancellation(t *testing.T) {
	cache, mini := newOpenAIWSHTTPFallbackTestCache(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	required, err := cache.OpenAIWSHTTPFallbackRequired(ctx, "session")
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, required)
	require.ErrorIs(t, cache.MarkOpenAIWSHTTPFallback(ctx, "session", time.Second), context.Canceled)
	require.Empty(t, mini.Keys())
}

func newOpenAIWSHTTPFallbackTestCache(t *testing.T) (*gatewayCache, *miniredis.Miniredis) {
	t.Helper()
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	return &gatewayCache{rdb: rdb}, mini
}
