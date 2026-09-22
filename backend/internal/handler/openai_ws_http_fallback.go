package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Call only before dispatch, or after route-v1 has validated non-execution.
// The client reconnects normally; its next handshake receives HTTP 426.
func (h *OpenAIGatewayHandler) prepareOpenAIWSHTTPFallback(c *gin.Context, sessionKey string, request service.OpenAIAccountScheduleRequest) bool {
	if sessionKey == "" {
		return false
	}
	log := requestLogger(c, "handler.openai_gateway.responses_ws")
	available, err := h.gatewayService.OpenAIWSHTTPFallbackAvailable(c.Request.Context(), request)
	if err != nil {
		log.Warn("openai.websocket_fallback_capability_lookup_failed", zap.Error(err))
		return false
	}
	if !available {
		return false
	}
	if err := h.gatewayService.MarkOpenAIWSHTTPFallback(c.Request.Context(), sessionKey); err != nil {
		log.Warn("openai.websocket_fallback_mark_failed", zap.Error(err))
		return false
	}
	log.Info("openai.websocket_http_fallback_prepared")
	return true
}
