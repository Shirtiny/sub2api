package handler

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectRequestControlHeadersPreservesCaseInsensitiveSessionSignals(t *testing.T) {
	projected := projectRequestControlHeaders(http.Header{
		"Session_Id":          {"standard-session"},
		"X-Aether-Session-Id": {"aether-session"},
		"X-Client-Request-Id": {"request-only-id"},
	})

	require.Equal(t, "standard-session", projected.Get("session_id"))
	require.Equal(t, "aether-session", projected.Get("x-aether-session-id"))
	require.Equal(t, "request-only-id", projected.Get("x-client-request-id"))
}
