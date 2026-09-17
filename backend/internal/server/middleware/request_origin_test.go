package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, peer, host, forwardedHost, xff, wantHost, wantIP string
	}{
		{"direct www", "203.0.113.9:1234", "WWW.CafeShop.AI:443", "", "", "www.cafeshop.ai", "203.0.113.9"},
		{"trusted NL double proxy", "172.20.0.1:4321", "www.cafeshop.ai", "nl.cafeshop.ai", "203.0.113.9, 162.211.230.96", "nl.cafeshop.ai", "203.0.113.9"},
		{"untrusted spoof", "203.0.113.9:1234", "www.cafeshop.ai", "nl.cafeshop.ai", "198.51.100.1", "www.cafeshop.ai", "203.0.113.9"},
		{"forged leftmost XFF", "172.20.0.1:1234", "www.cafeshop.ai", "nl.cafeshop.ai", "198.51.100.1, 203.0.113.9, 162.211.230.96", "nl.cafeshop.ai", "203.0.113.9"},
		{"invalid forwarded host", "172.20.0.1:1234", "www.cafeshop.ai", "https://nl.cafeshop.ai/path", "203.0.113.9", "www.cafeshop.ai", "203.0.113.9"},
		{"ambiguous host chain", "172.20.0.1:1234", "www.cafeshop.ai", "nl.cafeshop.ai, evil.example", "203.0.113.9", "www.cafeshop.ai", "203.0.113.9"},
		{"IPv4 mapped local peer", "[::ffff:172.20.0.1]:4321", "www.cafeshop.ai", "nl.cafeshop.ai", "203.0.113.9, 162.211.230.96", "nl.cafeshop.ai", "203.0.113.9"},
		{"trusted IPv6 proxy", "[::1]:4321", "www.cafeshop.ai", "nl.cafeshop.ai", "2001:db8::9", "nl.cafeshop.ai", "2001:db8::9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxies := []string{"172.20.0.1/32", "162.211.230.96/32", "::1"}
			r := gin.New()
			require.NoError(t, r.SetTrustedProxies(proxies))
			r.Use(RequestOrigin(proxies))
			r.GET("/v1/models", func(c *gin.Context) {
				require.Equal(t, tc.wantHost, httputil.RequestHost(c.Request))
				require.Equal(t, tc.wantIP, ip.GetTrustedClientIP(c))
				// Upstream rewrites must not alter the ingress snapshot.
				c.Request.Host = "upstream.internal"
				require.Equal(t, tc.wantHost, httputil.RequestHost(c.Request))
				c.Status(http.StatusOK)
			})
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			req.Host = tc.host
			req.RemoteAddr = tc.peer
			req.Header.Set("X-Forwarded-Host", tc.forwardedHost)
			req.Header.Set("X-Forwarded-For", tc.xff)
			req.Header.Set("CF-Connecting-IP", "198.51.100.77")
			req.Header.Set("X-Real-IP", "198.51.100.77")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestRequestOriginRejectsDuplicateForwardedHosts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestOrigin([]string{"127.0.0.1"}))
	r.GET("/", func(c *gin.Context) { require.Equal(t, "www.cafeshop.ai", httputil.RequestHost(c.Request)) })
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "www.cafeshop.ai"
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Add("X-Forwarded-Host", "nl.cafeshop.ai")
	req.Header.Add("X-Forwarded-Host", "evil.example")
	r.ServeHTTP(httptest.NewRecorder(), req)
}

func TestRequestOriginInvalidProxyConfigFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestOrigin([]string{"127.0.0.1", "not-a-network"}))
	r.GET("/", func(c *gin.Context) { require.Equal(t, "www.cafeshop.ai", httputil.RequestHost(c.Request)) })
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "www.cafeshop.ai"
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-Host", "nl.cafeshop.ai")
	r.ServeHTTP(httptest.NewRecorder(), req)
}
