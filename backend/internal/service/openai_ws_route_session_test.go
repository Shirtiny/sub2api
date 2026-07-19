package service

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveOpenAIWSRouteSessionIdentityCanonicalHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(7)
	body := []byte(`{"type":"response.create","client_metadata":{"session_id":"session-a","thread_id":"thread-a"}}`)
	c := newOpenAIWSRouteSessionTestContext()
	c.Request.Header.Set("session-id", "session-a")
	c.Request.Header.Set("thread-id", "thread-a")
	c.Request.Header.Set("x-client-request-id", "thread-a")

	identity := resolveOpenAIWSRouteSessionIdentityForTest(t, c, body, &groupID, 11, 13)
	require.True(t, identity.Reliable)
	require.False(t, identity.ProjectedHeaders)
	require.Equal(t, "canonical_headers", identity.Reason)
	require.Contains(t, identity.SessionKey, openAIWSRouteSessionHashPrefix)

	otherAuthScope := resolveOpenAIWSRouteSessionIdentityForTest(t, c, body, &groupID, 11, 14)
	require.NotEqual(t, identity.SessionKey, otherAuthScope.SessionKey)
}

func TestResolveOpenAIWSRouteSessionIdentityRejectsInconsistentSources(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(7)
	tests := map[string]struct {
		body            string
		sessionID       string
		threadID        string
		clientRequestID string
		wantReason      string
	}{
		"body session mismatch": {
			body:       `{"type":"response.create","client_metadata":{"session_id":"other","thread_id":"thread-a"}}`,
			sessionID:  "session-a",
			threadID:   "thread-a",
			wantReason: "session_identity_mismatch",
		},
		"body thread mismatch": {
			body:       `{"type":"response.create","client_metadata":{"session_id":"session-a","thread_id":"other"}}`,
			sessionID:  "session-a",
			threadID:   "thread-a",
			wantReason: "thread_identity_mismatch",
		},
		"x client mismatch": {
			body:            `{"type":"response.create","client_metadata":{"session_id":"session-a","thread_id":"thread-a"}}`,
			sessionID:       "session-a",
			threadID:        "thread-a",
			clientRequestID: "other",
			wantReason:      "client_request_thread_mismatch",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			c := newOpenAIWSRouteSessionTestContext()
			c.Request.Header.Set("session-id", test.sessionID)
			c.Request.Header.Set("thread-id", test.threadID)
			if test.clientRequestID != "" {
				c.Request.Header.Set("x-client-request-id", test.clientRequestID)
			}
			identity := resolveOpenAIWSRouteSessionIdentityForTest(t, c, []byte(test.body), &groupID, 11, 13)
			require.False(t, identity.Reliable)
			require.Equal(t, test.wantReason, identity.Reason)
		})
	}
}

func TestResolveOpenAIWSRouteSessionIdentityRejectsDuplicateCanonicalHeader(t *testing.T) {
	groupID := int64(7)
	c := newOpenAIWSRouteSessionTestContext()
	c.Request.Header.Add("session-id", "session-a")
	c.Request.Header.Add("session-id", "session-b")
	c.Request.Header.Set("thread-id", "thread-a")

	identity := resolveOpenAIWSRouteSessionIdentityForTest(t, c, []byte(`{"type":"response.create","client_metadata":{"session_id":"session-a","thread_id":"thread-a"}}`), &groupID, 11, 13)
	require.False(t, identity.Reliable)
	require.Equal(t, "identity_header_invalid", identity.Reason)
}

func TestResolveOpenAIWSRouteSessionIdentityBodyProjectionRequiresStrictOfficialFingerprint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(7)
	body := []byte(`{"type":"response.create","client_metadata":{"session_id":"session-a","thread_id":"thread-a"}}`)

	official := newOpenAIWSRouteSessionTestContext()
	official.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
	official.Request.Header.Set("originator", "codex_cli_rs")
	identity := resolveOpenAIWSRouteSessionIdentityForTest(t, official, body, &groupID, 11, 13)
	require.True(t, identity.Reliable)
	require.True(t, identity.ProjectedHeaders)
	require.Equal(t, "session-a", official.Request.Header.Get("session-id"))
	require.Equal(t, "thread-a", official.Request.Header.Get("thread-id"))

	nonOfficial := newOpenAIWSRouteSessionTestContext()
	nonOfficial.Request.Header.Set("User-Agent", "curl/8")
	nonOfficial.Request.Header.Set("originator", "codex_cli_rs")
	identity = resolveOpenAIWSRouteSessionIdentityForTest(t, nonOfficial, body, &groupID, 11, 13)
	require.False(t, identity.Reliable)
	require.Equal(t, "canonical_headers_missing", identity.Reason)

	containsSpoof := newOpenAIWSRouteSessionTestContext()
	containsSpoof.Request.Header.Set("User-Agent", "curl/8 codex_cli_rs/0.144.1")
	containsSpoof.Request.Header.Set("originator", "codex_cli_rs")
	identity = resolveOpenAIWSRouteSessionIdentityForTest(t, containsSpoof, body, &groupID, 11, 13)
	require.False(t, identity.Reliable)

	mismatchedRequestID := newOpenAIWSRouteSessionTestContext()
	mismatchedRequestID.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
	mismatchedRequestID.Request.Header.Set("originator", "codex_cli_rs")
	mismatchedRequestID.Request.Header.Set("x-client-request-id", "other-thread")
	identity = resolveOpenAIWSRouteSessionIdentityForTest(t, mismatchedRequestID, body, &groupID, 11, 13)
	require.False(t, identity.Reliable)
	require.Empty(t, mismatchedRequestID.Request.Header.Get("session-id"), "failed projection must not mutate headers")
	require.Empty(t, mismatchedRequestID.Request.Header.Get("thread-id"), "failed projection must not mutate headers")
}

func TestResolveOpenAIWSRouteSessionIdentityDoesNotUseContentOrUserFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(7)
	c := newOpenAIWSRouteSessionTestContext()
	identity := resolveOpenAIWSRouteSessionIdentityForTest(t, c, []byte(`{"type":"response.create","model":"gpt-5","input":"same content"}`), &groupID, 11, 13)
	require.False(t, identity.Reliable)
	require.Empty(t, identity.SessionKey)
}

func TestBuildOpenAIWSHeadersForwardsCanonicalCodexIdentity(t *testing.T) {
	c := newOpenAIWSRouteSessionTestContext()
	c.Request.Header.Set("session-id", "session-a")
	c.Request.Header.Set("thread-id", "thread-a")
	c.Request.Header.Set("x-client-request-id", "thread-a")

	svc := &OpenAIGatewayService{}
	headers, _ := svc.buildOpenAIWSHeaders(c, nil, "token", OpenAIWSProtocolDecision{}, true, "", "", "")
	require.Equal(t, "session-a", headers.Get("session-id"))
	require.Equal(t, "thread-a", headers.Get("thread-id"))
	require.Equal(t, "thread-a", headers.Get("x-client-request-id"))
}

func TestBuildOpenAIWSHeadersAppliesCafecodeIdentity(t *testing.T) {
	c := newOpenAIWSRouteSessionTestContext()
	c.Request.Header.Set("cafecode-uid", "spoofed")
	c.Request.Header.Set("cafecode-uname", "spoofed")
	c.Set("api_key", &APIKey{User: &User{
		ID:       42,
		Username: "cafe-user",
		Email:    "fallback@example.test",
	}})

	svc := &OpenAIGatewayService{}
	disabledHeaders, _ := svc.buildOpenAIWSHeaders(c, &Account{}, "token", OpenAIWSProtocolDecision{}, true, "", "", "")
	require.Empty(t, disabledHeaders.Get("cafecode-uid"))
	require.Empty(t, disabledHeaders.Get("cafecode-uname"))

	enabledAccount := &Account{Extra: map[string]any{
		openai_compat.ExtraKeyCafecodeIdentityHeadersEnabled: true,
	}}
	enabledHeaders, _ := svc.buildOpenAIWSHeaders(c, enabledAccount, "token", OpenAIWSProtocolDecision{}, true, "", "", "")
	require.Equal(t, "42", enabledHeaders.Get("cafecode-uid"))
	require.Equal(t, "cafe-user", enabledHeaders.Get("cafecode-uname"))
}

func newOpenAIWSRouteSessionTestContext() *gin.Context {
	c, _ := gin.CreateTestContext(nil)
	c.Request = httptest.NewRequest("GET", "/openai/v1/responses", nil)
	return c
}

func resolveOpenAIWSRouteSessionIdentityForTest(
	t *testing.T,
	c *gin.Context,
	body []byte,
	groupID *int64,
	userID int64,
	apiKeyID int64,
) OpenAIWSRouteSessionIdentity {
	t.Helper()
	envelope, err := ParseOpenAIWSClientEnvelope(body)
	require.NoError(t, err)
	return ResolveOpenAIWSRouteSessionIdentity(c, envelope, groupID, userID, apiKeyID)
}
