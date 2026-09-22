package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const openAIWSHTTPFallbackTTL = 30 * time.Second

// OpenAIWSFallbackCache shares short-lived transport decisions across gateway
// nodes. It is separate from account affinity and never consulted by frame relay.
type OpenAIWSFallbackCache interface {
	OpenAIWSHTTPFallbackRequired(ctx context.Context, sessionKey string) (bool, error)
	MarkOpenAIWSHTTPFallback(ctx context.Context, sessionKey string, ttl time.Duration) error
}

// ResolveOpenAIWSHTTPFallbackKey identifies only transport downgrade state. It
// does not authorize route-v1 migration or relax its first-frame identity proof.
// Resolve it before body projection changes the incoming handshake headers.
func ResolveOpenAIWSHTTPFallbackKey(c *gin.Context, groupID *int64, userID, apiKeyID int64) string {
	if c == nil || c.Request == nil || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	headers := c.Request.Header
	session, hasSession, validSession := singleOpenAIWSRouteIdentityHeader(headers, "session-id")
	thread, hasThread, validThread := singleOpenAIWSRouteIdentityHeader(headers, "thread-id")
	requestID, hasRequestID, validRequestID := singleOpenAIWSRouteIdentityHeader(headers, "x-client-request-id")
	if !validSession || !validThread || !validRequestID || (hasThread && hasRequestID && thread != requestID) {
		return ""
	}
	if hasSession && hasThread {
		return "http-fallback:" + hashOpenAIWSRouteSessionIdentity(groupID, userID, apiKeyID, session, thread)
	}
	// Some official builds only send the thread ID as x-client-request-id at
	// handshake time. Do not broaden the fallback scope to an entire API key.
	if !hasPinnedCodexCLIRouteIdentityFingerprint(headers) || !hasRequestID {
		return ""
	}
	return "http-fallback-header:" + hashOpenAIWSRouteSessionIdentity(groupID, userID, apiKeyID, "codex-thread", requestID)
}

func (s *OpenAIGatewayService) OpenAIWSHTTPFallbackRequired(ctx context.Context, sessionKey string) (bool, error) {
	if s == nil || sessionKey == "" {
		return false, nil
	}
	cache, ok := s.cache.(OpenAIWSFallbackCache)
	if !ok {
		return false, nil
	}
	return cache.OpenAIWSHTTPFallbackRequired(ctx, sessionKey)
}

func (s *OpenAIGatewayService) MarkOpenAIWSHTTPFallback(ctx context.Context, sessionKey string) error {
	if s == nil || sessionKey == "" {
		return errors.New("reliable websocket fallback identity is unavailable")
	}
	cache, ok := s.cache.(OpenAIWSFallbackCache)
	if !ok {
		return errors.New("shared websocket fallback cache is unavailable")
	}
	return cache.MarkOpenAIWSHTTPFallback(ctx, sessionKey, openAIWSHTTPFallbackTTL)
}

// OpenAIWSHTTPFallbackAvailable is a read-only, conservative capability check,
// not another scheduler. ExcludedIDs must contain only proven WS-unavailable
// routes (or exclusions admitted by route-v1), not arbitrary failed accounts.
// A positive result requires an HTTP candidate and no remaining WS candidate.
func (s *OpenAIGatewayService) OpenAIWSHTTPFallbackAvailable(ctx context.Context, req OpenAIAccountScheduleRequest) (bool, error) {
	if s == nil || (s.accountRepo == nil && s.schedulerSnapshot == nil) || req.Platform == PlatformGrok {
		return false, nil
	}
	if req.RequestedModel != "" && s.checkChannelPricingRestriction(ctx, req.GroupID, req.RequestedModel) {
		return false, nil
	}
	ctx = s.withOpenAIQuotaAutoPauseContext(ctx)
	accounts, err := s.listSchedulableAccounts(ctx, req.GroupID, req.Platform)
	if err != nil {
		return false, err
	}
	scheduler := defaultOpenAIAccountScheduler{service: s}
	hasHTTP := false
	for i := range accounts {
		account := &accounts[i]
		if !isOpenAICompatibleAccountEligibleForRequest(ctx, account, req.Platform, req.accountRoutingModel(), false, req.RequiredCapability) ||
			!scheduler.isAccountRequestCompatible(ctx, account, req) {
			continue
		}
		hasHTTP = true
		if _, excluded := req.ExcludedIDs[account.ID]; !excluded &&
			s.isOpenAIAccountTransportCompatible(account, OpenAIUpstreamTransportResponsesWebsocketV2) {
			return false, nil
		}
	}
	return hasHTTP, nil
}

// Only an actual handshake 426 (including validated Aether capability proof)
// marks an individual route as WS-unavailable. Other pre-dispatch errors are
// replay-safe but do not establish that HTTP is the appropriate transport.
func OpenAIWSFailoverRequiresHTTP(err *UpstreamFailoverError) bool {
	return err != nil && err.StatusCode == http.StatusUpgradeRequired &&
		err.MiddleRouteDisposition != OpenAIWSMiddleRouteDispositionRetain
}
