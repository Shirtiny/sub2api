package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	CafeCodeBrowserHistoryCookieName = "cfw_bh"

	browserHistoryCookieDomain       = ".cafecode.work"
	browserHistoryCookieMaxAgeSecond = 180 * 24 * 60 * 60
	browserHistoryCookieMaxLength    = 256
	browserHistorySignatureLength    = 16
	browserHistoryNonceBytes         = 12
	browserHistoryMaxVisitCount      = 99
	browserHistoryMaxUserIDs         = 8
	browserHistoryVersion            = "1"
	browserHistoryNoUserIDList       = "-"
	browserHistoryUserIDSeparator    = "~"
	browserHistoryMaxJWTLength       = 8192
)

var browserHistoryIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,24}$`)

// BrowserHistoryCookie maintains a compact, signed parent-domain cookie that can
// later be verified by store.cafecode.work as a weak browser-history signal.
func BrowserHistoryCookie(cfg *config.Config) gin.HandlerFunc {
	secret := browserHistoryCookieSecret(cfg)
	return func(c *gin.Context) {
		if shouldSkipBrowserHistoryCookie(c, secret) {
			c.Next()
			return
		}

		nowDay := unixDay(time.Now().UTC())
		currentUserID := currentCafeCodeUserID(c, cfg)
		value, refresh := nextBrowserHistoryCookieValue(c, secret, nowDay, currentUserID)
		if refresh {
			setBrowserHistoryCookie(c, value)
		}

		c.Next()
	}
}

func browserHistoryCookieSecret(cfg *config.Config) string {
	for _, value := range []string{
		os.Getenv("CAFE_BROWSER_COOKIE_SECRET"),
		os.Getenv("DEVICE_FINGERPRINT_SECRET"),
		os.Getenv("JWT_SECRET"),
	} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	if cfg != nil && (strings.EqualFold(cfg.Server.Mode, "debug") || gin.Mode() == gin.TestMode) {
		return strings.TrimSpace(cfg.JWT.Secret)
	}
	return ""
}

func shouldSkipBrowserHistoryCookie(c *gin.Context, secret string) bool {
	if strings.TrimSpace(secret) == "" || c == nil || c.Request == nil {
		return true
	}
	if c.Request.Method == http.MethodOptions || c.Request.Method == http.MethodHead {
		return true
	}
	if shouldSkipBrowserHistoryPath(c.Request.URL.Path) {
		return true
	}
	if !isCafeCodeCookieHost(browserHistoryRequestHost(c)) {
		return true
	}
	return false
}

func shouldSkipBrowserHistoryPath(path string) bool {
	path = strings.ToLower(strings.TrimSpace(path))
	if path == "" {
		return false
	}
	if strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/static/") || strings.HasPrefix(path, "/favicon") {
		return true
	}
	for _, suffix := range []string{
		".css", ".js", ".mjs", ".map", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico",
		".woff", ".woff2", ".ttf", ".otf", ".eot", ".txt", ".xml", ".json", ".webmanifest",
	} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func browserHistoryRequestHost(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if forwardedHost := firstForwardedHost(c.GetHeader("X-Forwarded-Host")); forwardedHost != "" {
		return forwardedHost
	}
	return c.Request.Host
}

func firstForwardedHost(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if index := strings.Index(value, ","); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}

func isCafeCodeCookieHost(hostport string) bool {
	host := strings.ToLower(strings.TrimSpace(hostport))
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	switch strings.TrimSuffix(host, ".") {
	case "cafecode.work", "www.cafecode.work":
		return true
	default:
		return false
	}
}

type browserHistoryJWTClaims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
}

func currentCafeCodeUserID(c *gin.Context, cfg *config.Config) int64 {
	if c == nil || cfg == nil || c.Request == nil {
		return 0
	}
	secret := strings.TrimSpace(cfg.JWT.Secret)
	if secret == "" {
		return 0
	}
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return 0
	}
	tokenString := strings.TrimSpace(parts[1])
	if tokenString == "" || len(tokenString) > browserHistoryMaxJWTLength {
		return 0
	}

	claims := &browserHistoryJWTClaims{}
	parsed, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected jwt signing method: %s", token.Method.Alg())
		}
		return []byte(secret), nil
	})
	if err != nil || parsed == nil || !parsed.Valid || claims.UserID <= 0 {
		return 0
	}
	return claims.UserID
}

func nextBrowserHistoryCookieValue(c *gin.Context, secret string, nowDay int64, currentUserID int64) (string, bool) {
	if raw, err := c.Request.Cookie(CafeCodeBrowserHistoryCookieName); err == nil {
		state, ok := parseBrowserHistoryCookie(raw.Value, secret)
		if ok {
			refresh := false
			if state.lastDay < nowDay {
				state.lastDay = nowDay
				if state.visitCount < browserHistoryMaxVisitCount {
					state.visitCount++
				}
				refresh = true
			}
			if merged, changed := mergeBrowserHistoryUserIDs(state.userIDs, currentUserID); changed {
				state.userIDs = merged
				refresh = true
			}
			if !refresh {
				return raw.Value, false
			}
			return signBrowserHistoryCookie(state, secret), true
		}
	}

	userIDs := []int64(nil)
	if currentUserID > 0 {
		userIDs = []int64{currentUserID}
	}
	id, err := newBrowserHistoryID()
	if err != nil {
		return "", false
	}
	return signBrowserHistoryCookie(browserHistoryCookieState{
		id:         id,
		firstDay:   nowDay,
		lastDay:    nowDay,
		visitCount: 1,
		userIDs:    userIDs,
	}, secret), true
}

func setBrowserHistoryCookie(c *gin.Context, value string) {
	if value == "" || len(value) > browserHistoryCookieMaxLength {
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CafeCodeBrowserHistoryCookieName,
		Value:    value,
		Domain:   browserHistoryCookieDomain,
		Path:     "/",
		MaxAge:   browserHistoryCookieMaxAgeSecond,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

type browserHistoryCookieState struct {
	id         string
	firstDay   int64
	lastDay    int64
	visitCount int64
	userIDs    []int64
}

func parseBrowserHistoryCookie(value string, secret string) (browserHistoryCookieState, bool) {
	if value == "" || len(value) > browserHistoryCookieMaxLength {
		return browserHistoryCookieState{}, false
	}
	parts := strings.Split(value, ".")
	if len(parts) != 6 && len(parts) != 7 {
		return browserHistoryCookieState{}, false
	}
	if parts[0] != browserHistoryVersion {
		return browserHistoryCookieState{}, false
	}
	id := parts[1]
	if !browserHistoryIDPattern.MatchString(id) {
		return browserHistoryCookieState{}, false
	}
	firstDay, ok := parseCompactDay(parts[2])
	if !ok {
		return browserHistoryCookieState{}, false
	}
	lastDay, ok := parseCompactDay(parts[3])
	if !ok || lastDay < firstDay {
		return browserHistoryCookieState{}, false
	}
	visitCount, ok := parseCompactVisitCount(parts[4])
	if !ok {
		return browserHistoryCookieState{}, false
	}

	signatureIndex := 5
	payloadParts := parts[:5]
	userIDs := []int64(nil)
	if len(parts) == 7 {
		var userIDOK bool
		userIDs, userIDOK = parseCompactUserIDs(parts[5])
		if !userIDOK {
			return browserHistoryCookieState{}, false
		}
		signatureIndex = 6
		payloadParts = parts[:6]
	}
	payload := strings.Join(payloadParts, ".")
	if !verifyBrowserHistorySignature(payload, parts[signatureIndex], secret) {
		return browserHistoryCookieState{}, false
	}
	return browserHistoryCookieState{id: id, firstDay: firstDay, lastDay: lastDay, visitCount: visitCount, userIDs: userIDs}, true
}

func parseCompactDay(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(value, 36, 64)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

func parseCompactVisitCount(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(value, 36, 64)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	if parsed > browserHistoryMaxVisitCount {
		parsed = browserHistoryMaxVisitCount
	}
	return parsed, true
}

func parseCompactUserIDs(value string) ([]int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" || value == browserHistoryNoUserIDList {
		return nil, true
	}
	ids := make([]int64, 0, browserHistoryMaxUserIDs)
	seen := make(map[int64]struct{}, browserHistoryMaxUserIDs)
	for _, part := range splitCompactUserIDs(value) {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		parsed, err := strconv.ParseInt(part, 36, 64)
		if err != nil || parsed <= 0 {
			return nil, false
		}
		if _, exists := seen[parsed]; exists {
			continue
		}
		seen[parsed] = struct{}{}
		if len(ids) < browserHistoryMaxUserIDs {
			ids = append(ids, parsed)
		}
	}
	return ids, true
}

func splitCompactUserIDs(value string) []string {
	if strings.Contains(value, browserHistoryUserIDSeparator) {
		return strings.Split(value, browserHistoryUserIDSeparator)
	}
	return strings.Split(value, ",")
}

func mergeBrowserHistoryUserIDs(existing []int64, current int64) ([]int64, bool) {
	if current <= 0 {
		return existing, false
	}
	merged := make([]int64, 0, browserHistoryMaxUserIDs)
	merged = append(merged, current)
	for _, id := range existing {
		if id <= 0 || id == current {
			continue
		}
		if len(merged) >= browserHistoryMaxUserIDs {
			break
		}
		merged = append(merged, id)
	}
	return merged, !sameInt64Slice(existing, merged)
}

func sameInt64Slice(a []int64, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func signBrowserHistoryCookie(state browserHistoryCookieState, secret string) string {
	payload := fmt.Sprintf("%s.%s.%s.%s.%s.%s",
		browserHistoryVersion,
		state.id,
		strconv.FormatInt(state.firstDay, 36),
		strconv.FormatInt(state.lastDay, 36),
		strconv.FormatInt(state.visitCount, 36),
		formatCompactUserIDs(state.userIDs),
	)
	return payload + "." + browserHistorySignature(payload, secret)
}

func formatCompactUserIDs(userIDs []int64) string {
	if len(userIDs) == 0 {
		return browserHistoryNoUserIDList
	}
	parts := make([]string, 0, min(len(userIDs), browserHistoryMaxUserIDs))
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		parts = append(parts, strconv.FormatInt(id, 36))
		if len(parts) >= browserHistoryMaxUserIDs {
			break
		}
	}
	if len(parts) == 0 {
		return browserHistoryNoUserIDList
	}
	return strings.Join(parts, browserHistoryUserIDSeparator)
}

func verifyBrowserHistorySignature(payload string, sig string, secret string) bool {
	expected := browserHistorySignature(payload, secret)
	if len(sig) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) == 1
}

func browserHistorySignature(payload string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))[:browserHistorySignatureLength]
}

func newBrowserHistoryID() (string, error) {
	buf := make([]byte, browserHistoryNonceBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func unixDay(t time.Time) int64 {
	return t.Unix() / int64((24 * time.Hour).Seconds())
}
