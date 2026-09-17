package httputil

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRequestHost(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"NL.CafeShop.AI:443", "nl.cafeshop.ai"}, {"www.cafeshop.ai.", "www.cafeshop.ai"},
		{"localhost:8080", "localhost"}, {"185.114.49.57:443", "185.114.49.57"},
		{"[2001:db8::1]:443", "2001:db8::1"}, {"[::1]", "::1"},
		{"https://nl.cafeshop.ai", ""}, {"evil.example/path", ""}, {"user@host", ""},
		{"host:invalid", ""}, {"host:65536", ""}, {"host:0", ""}, {"foo,bar", ""},
		{"foo..bar", ""}, {"-foo.example", ""}, {"foo_.example", ""},
		{strings.Repeat("x", 254), ""}, {"", ""},
	} {
		t.Run(tc.value, func(t *testing.T) { require.Equal(t, tc.want, NormalizeRequestHost(tc.value)) })
	}
}

func TestRequestHostWithoutMiddlewareIgnoresForwardedHost(t *testing.T) {
	req := httptest.NewRequest("GET", "https://www.cafeshop.ai/v1/models", nil)
	req.Header.Set("X-Forwarded-Host", "spoof.example")
	require.Equal(t, "www.cafeshop.ai", RequestHost(req))
	require.Empty(t, RequestHost(nil))
}
