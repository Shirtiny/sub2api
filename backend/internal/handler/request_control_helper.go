package handler

import (
	"fmt"
	"net/http"
	"strings"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (h *GatewayHandler) checkRequestControl(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte) *service.RequestControlDecision {
	if h == nil || h.requestControlService == nil {
		return nil
	}
	input := buildRequestControlInput(c, apiKey, subject, protocol, model, body)
	if protocol == service.RequestControlProtocolMessages {
		valid := service.IsClaudeCodeClient(c.Request.Context())
		input.ClaudeCodeValid = &valid
	}
	return runRequestControl(c, reqLog, h.requestControlService, input)
}

func (h *OpenAIGatewayHandler) checkRequestControl(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte) *service.RequestControlDecision {
	if h == nil || h.requestControlService == nil {
		return nil
	}
	input := buildRequestControlInput(c, apiKey, subject, protocol, model, body)
	return runRequestControl(c, reqLog, h.requestControlService, input)
}

func requestControlStatus(decision *service.RequestControlDecision) int {
	if decision == nil || decision.StatusCode < 400 || decision.StatusCode > 599 {
		return http.StatusForbidden
	}
	return decision.StatusCode
}

func requestControlErrorCode(_ *service.RequestControlDecision) string {
	return "request_control_violation"
}

func runRequestControl(c *gin.Context, reqLog *zap.Logger, svc *service.RequestControlService, input service.RequestControlCheckInput) *service.RequestControlDecision {
	if svc == nil || c == nil || c.Request == nil {
		return nil
	}
	decision, err := svc.Check(c.Request.Context(), input)
	if err != nil {
		if reqLog != nil {
			reqLog.Warn("request_control.check_failed", zap.Error(err))
		}
		return nil
	}
	if reqLog != nil && decision != nil && (decision.Blocked || decision.Observed) {
		reqLog.Info("request_control.decision", zap.String("action", decision.Action), zap.String("reason", decision.Reason), zap.Bool("blocked", decision.Blocked), zap.Bool("observed", decision.Observed), zap.String("client_kind", decision.ClientKind))
	}
	return decision
}

func buildRequestControlInput(c *gin.Context, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte) service.RequestControlCheckInput {
	input := service.RequestControlCheckInput{
		RequestID:      contentModerationRequestID(c.Request.Context()),
		UserID:         subject.UserID,
		Endpoint:       GetInboundEndpoint(c),
		Model:          strings.TrimSpace(model),
		Protocol:       protocol,
		Body:           body,
		Headers:        projectRequestControlHeaders(c.Request.Header),
		UserAgent:      strings.TrimSpace(c.GetHeader("User-Agent")),
		Originator:     strings.TrimSpace(c.GetHeader("originator")),
		TLSFingerprint: inboundTLSFingerprint(c),
		WebSocket:      isOpenAIWSUpgradeRequest(c.Request),
	}
	if input.Endpoint == "" {
		input.Endpoint = c.Request.URL.Path
	}
	if apiKey != nil {
		input.APIKeyID = apiKey.ID
		input.APIKeyName = apiKey.Name
		if apiKey.User != nil {
			input.UserEmail = apiKey.User.Email
		}
		if apiKey.GroupID != nil {
			value := *apiKey.GroupID
			input.GroupID = &value
		}
		if apiKey.Group != nil {
			input.GroupName = apiKey.Group.Name
			input.Provider = strings.TrimSpace(apiKey.Group.Platform)
			if apiKey.Group.CustomSourceGroupID != nil && *apiKey.Group.CustomSourceGroupID > 0 {
				value := *apiKey.Group.CustomSourceGroupID
				if input.GroupID == nil || value != *input.GroupID {
					input.EffectiveGroupIDs = append(input.EffectiveGroupIDs, value)
				}
			}
		}
	}
	return input
}

func projectRequestControlHeaders(source http.Header) http.Header {
	projected := make(http.Header, 16)
	for _, name := range []string{
		"User-Agent",
		"originator",
		"Accept",
		"Content-Type",
		"session-id",
		"thread-id",
		"x-client-request-id",
		"x-codex-turn-metadata",
		"x-codex-beta-features",
		"OpenAI-Beta",
		"X-App",
		"anthropic-beta",
		"anthropic-version",
		"X-Claude-Code-Session-Id",
	} {
		for _, value := range source.Values(name) {
			projected.Add(name, value)
		}
	}
	return projected
}

func inboundTLSFingerprint(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	// Proxy-provided values are diagnostic only. Request control never uses
	// them for allow/block decisions because this layer cannot attest them.
	for _, name := range []string{
		"X-Aether-TLS-JA3-Hash",
		"X-Aether-TLS-JA4",
		"CF-JA3-Hash",
		"X-TLS-Fingerprint",
		"X-Client-TLS-Fingerprint",
		"X-TLS-Client-JA3",
		"X-Aether-TLS-JA3",
	} {
		if value := strings.TrimSpace(c.GetHeader(name)); value != "" {
			return "proxy:" + strings.ToLower(name) + "=" + value
		}
	}
	if c.Request.TLS != nil {
		return fmt.Sprintf(
			"direct:version=%04x;cipher=%04x;alpn=%s",
			c.Request.TLS.Version,
			c.Request.TLS.CipherSuite,
			strings.TrimSpace(c.Request.TLS.NegotiatedProtocol),
		)
	}
	return ""
}
