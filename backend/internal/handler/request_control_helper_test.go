package handler

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
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
