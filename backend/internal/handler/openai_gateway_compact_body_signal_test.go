package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

func newCompactBodySignalTestContext(t *testing.T, path string, body []byte) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func TestNormalizeOpenAIResponsesCompactRequest_BodySignalNotPromoted(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{
		"model":"gpt-5.5",
		"stream":true,
		"store":true,
		"prompt_cache_key":"pck-signal-1",
		"input":[
			{"type":"message","role":"user","content":"hello"},
			{"type":"compaction_trigger"}
		]
	}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses", body)
	c.Set(ctxKeyInboundEndpoint, EndpointResponses)

	normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)

	require.Equal(t, "/v1/responses", c.Request.URL.Path)
	require.False(t, isOpenAIRemoteCompactPath(c))
	require.Equal(t, EndpointResponses, GetInboundEndpoint(c))
	require.Equal(t, body, normalized)

	require.True(t, gjson.GetBytes(normalized, "stream").Bool())
	require.True(t, gjson.GetBytes(normalized, "store").Bool())
	require.Equal(t, "pck-signal-1", gjson.GetBytes(normalized, "prompt_cache_key").String())
	require.Equal(t, "gpt-5.5", gjson.GetBytes(normalized, "model").String())
	require.Equal(t, "compaction_trigger", gjson.GetBytes(normalized, "input.1.type").String())

	reqStream, streamOK := parseOpenAICompatibleStream(normalized)
	require.True(t, streamOK)
	require.True(t, reqStream)

	_, exists := c.Get(service.OpenAICompactSessionSeedKeyForTest())
	require.False(t, exists)
}

func TestNormalizeOpenAIResponsesCompactRequest_BodySignalTrailingSlash(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses/", body)

	normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	require.Equal(t, "/v1/responses/", c.Request.URL.Path)
	require.False(t, isOpenAIRemoteCompactPath(c))
	require.Equal(t, body, normalized)
}

func TestNormalizeOpenAIResponsesCompactRequest_CodexDirectAliasNotPromoted(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`)
	c := newCompactBodySignalTestContext(t, "/backend-api/codex/responses", body)

	normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	require.Equal(t, "/backend-api/codex/responses", c.Request.URL.Path)
	require.False(t, isOpenAIRemoteCompactPath(c))
	require.Equal(t, body, normalized)
}

func TestNormalizeOpenAIResponsesCompactRequest_NoTriggerUntouched(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"type":"message","role":"user","content":"hello"}]}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses", body)
	c.Set(ctxKeyInboundEndpoint, EndpointResponses)

	normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	require.Equal(t, "/v1/responses", c.Request.URL.Path)
	require.False(t, isOpenAIRemoteCompactPath(c))
	require.Equal(t, EndpointResponses, GetInboundEndpoint(c))
	require.Equal(t, body, normalized)
	require.True(t, gjson.GetBytes(normalized, "stream").Bool())
}

func TestNormalizeOpenAIResponsesCompactRequest_PathBasedNoDoubleSuffix(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","stream":true,"store":true,"input":[{"type":"message","role":"user","content":"hello"}]}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses/compact", body)
	c.Set(ctxKeyInboundEndpoint, EndpointResponses)

	normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	require.Equal(t, "/v1/responses/compact", c.Request.URL.Path)
	require.Equal(t, EndpointResponsesCompact, GetInboundEndpoint(c))
	require.True(t, gjson.GetBytes(normalized, "stream").Bool())
	require.False(t, gjson.GetBytes(normalized, "store").Exists())
}

func TestNormalizeOpenAIResponsesCompactRequest_SubpathNotPromoted(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses/resp_123/cancel", body)

	normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	require.Equal(t, "/v1/responses/resp_123/cancel", c.Request.URL.Path)
	require.Equal(t, body, normalized)
}

func TestEnsureOpenAICompactionRoutingSessionInjectsStableIsolatedUUID(t *testing.T) {
	first := newCompactBodySignalTestContext(t, "/v1/responses", []byte(`{}`))
	require.True(t, ensureOpenAICompactionRoutingSession(first, 17, "session-hash"))
	firstSession := first.GetHeader("session_id")
	require.NotEmpty(t, firstSession)
	_, err := uuid.Parse(firstSession)
	require.NoError(t, err)
	require.False(t, ensureOpenAICompactionRoutingSession(first, 17, "session-hash"))
	require.Equal(t, firstSession, first.GetHeader("session_id"))

	sameKey := newCompactBodySignalTestContext(t, "/v1/responses", []byte(`{}`))
	require.True(t, ensureOpenAICompactionRoutingSession(sameKey, 17, "session-hash"))
	require.Equal(t, firstSession, sameKey.GetHeader("session_id"))

	differentKey := newCompactBodySignalTestContext(t, "/v1/responses", []byte(`{}`))
	require.True(t, ensureOpenAICompactionRoutingSession(differentKey, 18, "session-hash"))
	require.NotEqual(t, firstSession, differentKey.GetHeader("session_id"))
}

func TestEnsureOpenAICompactionRoutingSessionPreservesClientIdentity(t *testing.T) {
	withSession := newCompactBodySignalTestContext(t, "/v1/responses", []byte(`{}`))
	withSession.Request.Header.Set("session_id", "client-session")
	require.False(t, ensureOpenAICompactionRoutingSession(withSession, 17, "session-hash"))
	require.Equal(t, "client-session", withSession.GetHeader("session_id"))

	withConversation := newCompactBodySignalTestContext(t, "/v1/responses", []byte(`{}`))
	withConversation.Request.Header.Set("conversation_id", "client-conversation")
	require.False(t, ensureOpenAICompactionRoutingSession(withConversation, 17, "session-hash"))
	require.Empty(t, withConversation.GetHeader("session_id"))
}
