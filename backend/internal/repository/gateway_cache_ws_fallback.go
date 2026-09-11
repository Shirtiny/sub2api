package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const openAIWSHTTPFallbackPrefix = "openai_ws_http_fallback:"

var _ service.OpenAIWSFallbackCache = (*gatewayCache)(nil)

func (c *gatewayCache) OpenAIWSHTTPFallbackRequired(ctx context.Context, sessionKey string) (bool, error) {
	if strings.TrimSpace(sessionKey) == "" {
		return false, errors.New("openai websocket fallback session key is empty")
	}
	exists, err := c.rdb.Exists(ctx, openAIWSHTTPFallbackPrefix+sessionKey).Result()
	return exists > 0, err
}

func (c *gatewayCache) MarkOpenAIWSHTTPFallback(ctx context.Context, sessionKey string, ttl time.Duration) error {
	if strings.TrimSpace(sessionKey) == "" {
		return errors.New("openai websocket fallback session key is empty")
	}
	if ttl <= 0 {
		return errors.New("openai websocket fallback ttl must be positive")
	}
	// Repeated failures must not extend the original downgrade window.
	return c.rdb.SetNX(ctx, openAIWSHTTPFallbackPrefix+sessionKey, "1", ttl).Err()
}
