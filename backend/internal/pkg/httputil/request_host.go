package httputil

import (
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// RequestHost returns the ingress host resolved by trusted-proxy middleware.
// Without middleware, only the actual HTTP Host is used, never forwarding headers.
func RequestHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	if host, ok := r.Context().Value(ctxkey.RequestHost).(string); ok {
		return host
	}
	return NormalizeRequestHost(r.Host)
}

// NormalizeRequestHost stores a hostname, not a URL, path, port or proxy chain.
func NormalizeRequestHost(value string) string {
	host := strings.ToLower(strings.TrimSpace(value))
	if host == "" || strings.ContainsAny(host, "/\\@?#,% \t\r\n") {
		return ""
	}
	if h, port, err := net.SplitHostPort(host); err == nil {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return ""
		}
		host = h
	} else if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap().String()
	}
	host = strings.TrimSuffix(host, ".")
	if len(host) == 0 || len(host) > 253 {
		return ""
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return ""
		}
		for _, ch := range label {
			if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' {
				return ""
			}
		}
	}
	return host
}
