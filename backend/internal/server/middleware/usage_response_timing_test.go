package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUsageResponseTimingUsesUpstreamObservations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(UsageResponseTiming())

	var firstByteMs *int
	var duration time.Duration
	var durationOK bool
	router.GET("/", func(c *gin.Context) {
		startedAt := time.Now().Add(-100 * time.Millisecond)
		BeginUsageResponseTiming(c, startedAt)
		timing := service.UsageResponseTimingFromContext(c.Request.Context())
		require.NotNil(t, timing)

		timing.ObserveUpstreamRead(startedAt.Add(20 * time.Millisecond))
		timing.ObserveUpstreamRead(startedAt.Add(75 * time.Millisecond))
		firstByteMs = UsageResponseFirstByteMs(c)
		duration, durationOK = UsageResponseUpstreamDuration(c)

		c.String(http.StatusOK, "ok")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/", nil),
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotNil(t, firstByteMs)
	require.Equal(t, 20, *firstByteMs)
	require.True(t, durationOK)
	require.Equal(t, 75*time.Millisecond, duration)
}

func TestUsageResponseTimingAttachesPreDerivedContext(t *testing.T) {
	type contextKey struct{}
	base := context.WithValue(
		context.Background(),
		contextKey{},
		"preserved",
	)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	BeginUsageResponseTiming(c, time.Now())
	derived := WithUsageResponseTiming(c, base)

	require.Equal(t, "preserved", derived.Value(contextKey{}))
	require.NotNil(t, service.UsageResponseTimingFromContext(derived))
}

func TestUsageResponseTimingBeginIsolatesAttempts(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	firstStart := time.Now().Add(-time.Second)
	BeginUsageResponseTiming(c, firstStart)
	first := service.UsageResponseTimingFromContext(c.Request.Context())
	first.ObserveUpstreamRead(firstStart.Add(100 * time.Millisecond))
	require.NotNil(t, UsageResponseFirstByteMs(c))

	BeginUsageResponseTiming(c, time.Now())
	second := service.UsageResponseTimingFromContext(c.Request.Context())
	require.NotSame(t, first, second)
	require.Nil(t, UsageResponseFirstByteMs(c))
}
