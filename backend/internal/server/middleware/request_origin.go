package middleware

import (
	"context"
	"net/netip"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
)

// RequestOrigin snapshots the external hostname before handlers rewrite requests.
// Proxy administrators must overwrite X-Forwarded-Host at the public edge.
func RequestOrigin(trustedProxies []string) gin.HandlerFunc {
	prefixes := make([]netip.Prefix, 0, len(trustedProxies))
	for _, value := range trustedProxies {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			addr, addrErr := netip.ParseAddr(value)
			if addrErr != nil {
				// Fail closed, matching an invalid proxy configuration rather than trusting a subset.
				prefixes = nil
				break
			}
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}
		prefixes = append(prefixes, prefix)
	}
	return func(c *gin.Context) {
		host := httputil.NormalizeRequestHost(c.Request.Host)
		peer, err := netip.ParseAddr(c.RemoteIP())
		if err == nil {
			for _, prefix := range prefixes {
				if !prefix.Contains(peer.Unmap()) {
					continue
				}
				values := c.Request.Header.Values("X-Forwarded-Host")
				if len(values) == 1 && !strings.Contains(values[0], ",") {
					if forwarded := httputil.NormalizeRequestHost(values[0]); forwarded != "" {
						host = forwarded
					}
				}
				break
			}
		}
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.RequestHost, host))
		c.Next()
	}
}
