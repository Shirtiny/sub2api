package handler

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const cyberPolicyRecordedKey = "cyber_policy_recorded"

// recordCyberPolicyIfMarked snapshots the authenticated user context before
// returning to the request loop, then performs risk-control persistence in a
// bounded background task. The upstream response is already on the wire.
func (h *OpenAIGatewayHandler) recordCyberPolicyIfMarked(c *gin.Context, apiKey *service.APIKey, account *service.Account, model string) {
	mark := service.GetOpsCyberPolicy(c)
	if mark == nil || h == nil || h.contentModerationService == nil || c.GetBool(cyberPolicyRecordedKey) {
		return
	}
	c.Set(cyberPolicyRecordedKey, true)
	var userID int64
	var userEmail string
	var apiKeyID int64
	var apiKeyName, groupName string
	var groupID *int64
	if apiKey != nil {
		apiKeyID = apiKey.ID
		apiKeyName = apiKey.Name
		groupID = apiKey.GroupID
		if apiKey.User != nil {
			userID = apiKey.User.ID
			userEmail = apiKey.User.Email
		}
		if apiKey.Group != nil {
			groupName = apiKey.Group.Name
		}
	}
	provider := service.PlatformOpenAI
	if account != nil && strings.TrimSpace(account.Platform) != "" {
		provider = account.Platform
	}
	requestID := ""
	if c.Writer != nil {
		requestID = c.Writer.Header().Get("X-Request-Id")
	}
	input := service.CyberPolicyRecordInput{
		RequestID:       requestID,
		UserID:          userID,
		UserEmail:       userEmail,
		APIKeyID:        apiKeyID,
		APIKeyName:      apiKeyName,
		GroupID:         groupID,
		GroupName:       groupName,
		Endpoint:        GetInboundEndpoint(c),
		Provider:        provider,
		Model:           strings.TrimSpace(model),
		UpstreamMessage: mark.Message,
		UpstreamBody:    mark.Body,
		UpstreamStatus:  mark.UpstreamStatus,
	}
	svc := h.contentModerationService
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		svc.RecordCyberPolicyEvent(ctx, input)
	}()
}

func clearCyberPolicyTurnState(c *gin.Context) {
	if c == nil {
		return
	}
	service.ClearOpsCyberPolicy(c)
	c.Set(cyberPolicyRecordedKey, false)
}
