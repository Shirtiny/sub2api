package handler

import (
	"context"
	"time"

	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func beginUsageResponseTiming(c *gin.Context, startedAt time.Time, derivedContexts ...context.Context) context.Context {
	servermiddleware.BeginUsageResponseTiming(c, startedAt)
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	if len(derivedContexts) > 0 && derivedContexts[0] != nil {
		ctx = derivedContexts[0]
	}
	return servermiddleware.WithUsageResponseTiming(c, ctx)
}

func finishUsageResponseTiming(c *gin.Context, startedAt time.Time, result *service.ForwardResult) {
	if result == nil {
		return
	}
	if duration, ok := servermiddleware.UsageResponseUpstreamDuration(c); ok {
		result.Duration = duration
	} else if result.Duration <= 0 {
		result.Duration = time.Since(startedAt)
	}
	if firstByteMs := servermiddleware.UsageResponseFirstByteMs(c); firstByteMs != nil {
		result.FirstByteMs = firstByteMs
	}
}

func finishOpenAIUsageResponseTiming(c *gin.Context, startedAt time.Time, result *service.OpenAIForwardResult) {
	if result == nil {
		return
	}
	if duration, ok := servermiddleware.UsageResponseUpstreamDuration(c); ok {
		result.Duration = duration
	} else if result.Duration <= 0 {
		result.Duration = time.Since(startedAt)
	}
	if firstByteMs := servermiddleware.UsageResponseFirstByteMs(c); firstByteMs != nil {
		result.FirstByteMs = firstByteMs
	}
}
