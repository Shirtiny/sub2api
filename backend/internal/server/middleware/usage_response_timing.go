package middleware

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const usageResponseTimingContextKey = "usage_response_timing"

// UsageResponseTiming installs the request-scoped holder used by gateway
// handlers and the upstream HTTP transport. It does not observe downstream
// writes; usage latency is measured exclusively at upstream read boundaries.
func UsageResponseTiming() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c == nil {
			return
		}
		c.Set(usageResponseTimingContextKey, (*service.UsageResponseTiming)(nil))
		c.Next()
	}
}

// BeginUsageResponseTiming creates an isolated tracker for one forwarding
// attempt and attaches it to the Gin request context.
func BeginUsageResponseTiming(c *gin.Context, startedAt time.Time) {
	if c == nil {
		return
	}
	timing := service.NewUsageResponseTiming(startedAt)
	c.Set(usageResponseTimingContextKey, timing)
	if c.Request != nil {
		ctx := service.WithUsageResponseTiming(c.Request.Context(), timing)
		c.Request = c.Request.WithContext(ctx)
	}
}

// WithUsageResponseTiming attaches the current attempt tracker to a context
// that was derived before BeginUsageResponseTiming ran.
func WithUsageResponseTiming(c *gin.Context, ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil {
		return ctx
	}
	value, ok := c.Get(usageResponseTimingContextKey)
	if !ok {
		return ctx
	}
	timing, _ := value.(*service.UsageResponseTiming)
	return service.WithUsageResponseTiming(ctx, timing)
}

func UsageResponseFirstByteMs(c *gin.Context) *int {
	if c == nil {
		return nil
	}
	value, ok := c.Get(usageResponseTimingContextKey)
	if !ok {
		return nil
	}
	timing, _ := value.(*service.UsageResponseTiming)
	return timing.FirstByteMs()
}

func UsageResponseUpstreamDuration(c *gin.Context) (time.Duration, bool) {
	if c == nil {
		return 0, false
	}
	value, ok := c.Get(usageResponseTimingContextKey)
	if !ok {
		return 0, false
	}
	timing, _ := value.(*service.UsageResponseTiming)
	return timing.UpstreamDuration()
}
