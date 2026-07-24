package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFinishOpenAIUsageResponseTimingUsesUpstreamBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	startedAt := time.Now().Add(-time.Second)

	ctx := beginUsageResponseTiming(c, startedAt)
	timing := service.UsageResponseTimingFromContext(ctx)
	require.NotNil(t, timing)
	timing.ObserveUpstreamRead(startedAt.Add(120 * time.Millisecond))
	timing.ObserveUpstreamRead(startedAt.Add(640 * time.Millisecond))

	firstByteMs := 900
	result := &service.OpenAIForwardResult{
		Duration:    5 * time.Second,
		FirstByteMs: &firstByteMs,
	}
	finishOpenAIUsageResponseTiming(c, startedAt, result)

	require.Equal(t, 640*time.Millisecond, result.Duration)
	require.NotNil(t, result.FirstByteMs)
	require.Equal(t, 120, *result.FirstByteMs)
}

func TestFinishOpenAIUsageResponseTimingPreservesWSTimingWithoutHTTPRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	startedAt := time.Now().Add(-time.Second)
	beginUsageResponseTiming(c, startedAt)

	firstByteMs := 80
	result := &service.OpenAIForwardResult{
		Duration:    700 * time.Millisecond,
		FirstByteMs: &firstByteMs,
	}
	finishOpenAIUsageResponseTiming(c, startedAt, result)

	require.Equal(t, 700*time.Millisecond, result.Duration)
	require.NotNil(t, result.FirstByteMs)
	require.Equal(t, 80, *result.FirstByteMs)
}
