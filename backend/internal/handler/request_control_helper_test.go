package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProjectRequestControlHeadersPreservesCaseInsensitiveSessionSignals(t *testing.T) {
	projected := projectRequestControlHeaders(http.Header{
		"Session_Id":              {"standard-session"},
		"X-Aether-Session-Id":     {"aether-session"},
		"X-Client-Request-Id":     {"request-only-id"},
		"X-Codex-Installation-Id": {"installation-id"},
	})

	require.Equal(t, "standard-session", projected.Get("session_id"))
	require.Equal(t, "aether-session", projected.Get("x-aether-session-id"))
	require.Equal(t, "request-only-id", projected.Get("x-client-request-id"))
	require.Equal(t, "installation-id", projected.Get("x-codex-installation-id"))
}

func TestBuildRequestControlInputCapturesRequestContextForAdminSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "https://www.cafecode.work/v1/responses?debug=1", nil)
	c.Request.RemoteAddr = "10.0.0.2:4321"
	c.Request.ContentLength = 123
	c.Request.Header.Set("X-Debug", "trace")

	input := buildRequestControlInput(c, nil, middleware2.AuthSubject{UserID: 42}, service.RequestControlProtocolResponse, "gpt-5", []byte(`{"model":"gpt-5"}`))

	require.Equal(t, http.MethodPost, input.RequestMethod)
	require.Equal(t, "www.cafecode.work", input.RequestHost)
	require.Equal(t, "/v1/responses", input.RequestPath)
	require.Equal(t, "debug=1", input.RequestQuery)
	require.Equal(t, "10.0.0.2:4321", input.RemoteAddr)
	require.Equal(t, int64(123), input.ContentLength)
	require.Equal(t, "trace", input.MetadataHeaders.Get("X-Debug"))
}

func TestRequestControlClientBlockResponseDoesNotExposeInternalReason(t *testing.T) {
	decision := &service.RequestControlDecision{StatusCode: 429, Reason: "codex_request_signature_mismatch", Message: "custom policy prompt"}

	require.Equal(t, http.StatusTooManyRequests, requestControlStatus(decision))
	require.Equal(t, "403", requestControlErrorCode(decision))
	require.Equal(t, "custom policy prompt", requestControlClientMessage(decision))
	require.NotContains(t, requestControlClientMessage(decision), decision.Reason)
}

func TestRequestControlClientBlockResponseUsesDefaultsWhenDecisionValuesAreInvalid(t *testing.T) {
	decision := &service.RequestControlDecision{StatusCode: 200}

	require.Equal(t, http.StatusForbidden, requestControlStatus(decision))
	require.Equal(t, "403", requestControlErrorCode(decision))
	require.Equal(t, "该请求已被限制，请使用codex重新请求", requestControlClientMessage(decision))
}
