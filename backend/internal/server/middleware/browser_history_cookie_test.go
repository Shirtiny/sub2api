package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func clearBrowserHistoryCookieSecretEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CAFE_BROWSER_COOKIE_SECRET", "")
	t.Setenv("DEVICE_FINGERPRINT_SECRET", "")
	t.Setenv("JWT_SECRET", "")
}

func testBearerToken(t *testing.T, secret string, userID int64) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return "Bearer " + signed
}

func TestBrowserHistoryCookieSetsCompactParentDomainCookie(t *testing.T) {
	clearBrowserHistoryCookieSecretEnv(t)
	gin.SetMode(gin.TestMode)

	secret := "test-shared-cookie-secret-32bytes"
	r := gin.New()
	r.Use(BrowserHistoryCookie(&config.Config{JWT: config.JWTConfig{Secret: secret}}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://www.cafecode.work/", nil)
	req.Header.Set("Authorization", testBearerToken(t, secret, 12345))
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	ck := cookies[0]
	require.Equal(t, CafeCodeBrowserHistoryCookieName, ck.Name)
	require.Equal(t, "cafecode.work", ck.Domain)
	require.Equal(t, "/", ck.Path)
	require.True(t, ck.HttpOnly)
	require.True(t, ck.Secure)
	require.Equal(t, http.SameSiteLaxMode, ck.SameSite)
	require.LessOrEqual(t, len(ck.Value), 160)

	state, ok := parseBrowserHistoryCookie(ck.Value, secret)
	require.True(t, ok)
	require.NotEmpty(t, state.id)
	require.EqualValues(t, 1, state.visitCount)
	require.Equal(t, []int64{12345}, state.userIDs)
}

func TestBrowserHistoryCookieAllowsSeparateCookieAndJWTSecrets(t *testing.T) {
	clearBrowserHistoryCookieSecretEnv(t)
	gin.SetMode(gin.TestMode)

	cookieSecret := "test-shared-cookie-secret-32bytes"
	jwtSecret := "test-jwt-secret-32bytes"
	t.Setenv("CAFE_BROWSER_COOKIE_SECRET", cookieSecret)

	r := gin.New()
	r.Use(BrowserHistoryCookie(&config.Config{JWT: config.JWTConfig{Secret: jwtSecret}}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://www.cafecode.work/", nil)
	req.Header.Set("Authorization", testBearerToken(t, jwtSecret, 44))
	r.ServeHTTP(w, req)

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	state, ok := parseBrowserHistoryCookie(cookies[0].Value, cookieSecret)
	require.True(t, ok)
	require.Equal(t, []int64{44}, state.userIDs)

	_, ok = parseBrowserHistoryCookie(cookies[0].Value, jwtSecret)
	require.False(t, ok)
}

func TestBrowserHistoryCookieRefreshesAtMostOncePerDay(t *testing.T) {
	clearBrowserHistoryCookieSecretEnv(t)
	secret := "test-shared-cookie-secret-32bytes"
	yesterday := unixDay(time.Now().UTC()) - 1
	oldValue := signBrowserHistoryCookie(browserHistoryCookieState{
		id:         "abcdefghijklmnop",
		firstDay:   yesterday,
		lastDay:    yesterday,
		visitCount: 1,
		userIDs:    []int64{11},
	}, secret)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BrowserHistoryCookie(&config.Config{JWT: config.JWTConfig{Secret: secret}}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://cafecode.work/", nil)
	req.AddCookie(&http.Cookie{Name: CafeCodeBrowserHistoryCookieName, Value: oldValue})
	req.Header.Set("Authorization", testBearerToken(t, secret, 22))
	r.ServeHTTP(w, req)

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	state, ok := parseBrowserHistoryCookie(cookies[0].Value, secret)
	require.True(t, ok)
	require.Equal(t, "abcdefghijklmnop", state.id)
	require.EqualValues(t, 2, state.visitCount)
	require.Equal(t, []int64{22, 11}, state.userIDs)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "https://cafecode.work/", nil)
	req.AddCookie(&http.Cookie{Name: CafeCodeBrowserHistoryCookieName, Value: cookies[0].Value})
	req.Header.Set("Authorization", testBearerToken(t, secret, 22))
	r.ServeHTTP(w, req)
	require.Empty(t, w.Result().Cookies())
}

func TestBrowserHistoryCookieUpgradesLegacyCookieWhenUserIDAppears(t *testing.T) {
	clearBrowserHistoryCookieSecretEnv(t)
	secret := "test-shared-cookie-secret-32bytes"
	today := unixDay(time.Now().UTC())
	legacyPayload := strings.Join([]string{"1", "abcdefghijklmnop", "" + formatBase36(today), "" + formatBase36(today), "1"}, ".")
	legacyValue := legacyPayload + "." + browserHistorySignature(legacyPayload, secret)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BrowserHistoryCookie(&config.Config{JWT: config.JWTConfig{Secret: secret}}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://cafecode.work/", nil)
	req.AddCookie(&http.Cookie{Name: CafeCodeBrowserHistoryCookieName, Value: legacyValue})
	req.Header.Set("Authorization", testBearerToken(t, secret, 33))
	r.ServeHTTP(w, req)

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	state, ok := parseBrowserHistoryCookie(cookies[0].Value, secret)
	require.True(t, ok)
	require.Equal(t, []int64{33}, state.userIDs)
}

func TestBrowserHistoryCookieUsesForwardedHost(t *testing.T) {
	clearBrowserHistoryCookieSecretEnv(t)
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(BrowserHistoryCookie(&config.Config{JWT: config.JWTConfig{Secret: "test-shared-cookie-secret-32bytes"}}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://internal.local/", nil)
	req.Header.Set("X-Forwarded-Host", "www.cafecode.work, internal.local")
	r.ServeHTTP(w, req)

	require.Len(t, w.Result().Cookies(), 1)
}

func TestBrowserHistoryCookieSkipsStaticAssets(t *testing.T) {
	clearBrowserHistoryCookieSecretEnv(t)
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(BrowserHistoryCookie(&config.Config{JWT: config.JWTConfig{Secret: "test-shared-cookie-secret-32bytes"}}))
	r.GET("/assets/app.js", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://www.cafecode.work/assets/app.js", nil)
	r.ServeHTTP(w, req)

	require.Empty(t, w.Result().Cookies())
}

func TestBrowserHistoryCookieSkipsOtherHostsAndMissingSecret(t *testing.T) {
	clearBrowserHistoryCookieSecretEnv(t)
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name string
		cfg  *config.Config
		host string
	}{
		{name: "other_host", cfg: &config.Config{JWT: config.JWTConfig{Secret: "test-shared-cookie-secret-32bytes"}}, host: "store.cafecode.work"},
		{name: "missing_secret", cfg: &config.Config{}, host: "www.cafecode.work"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(BrowserHistoryCookie(tc.cfg))
			r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "https://"+tc.host+"/", nil)
			r.ServeHTTP(w, req)

			require.Empty(t, w.Result().Cookies())
		})
	}
}

func TestBrowserHistoryCookieUsesCookieSafeUserIDSeparator(t *testing.T) {
	secret := "test-shared-cookie-secret-32bytes"
	value := signBrowserHistoryCookie(browserHistoryCookieState{
		id:         "abcdefghijklmnop",
		firstDay:   20000,
		lastDay:    20000,
		visitCount: 1,
		userIDs:    []int64{35, 36},
	}, secret)

	require.Contains(t, value, ".z~10.")
	require.NotContains(t, value, ",")
	state, ok := parseBrowserHistoryCookie(value, secret)
	require.True(t, ok)
	require.Equal(t, []int64{35, 36}, state.userIDs)

	legacyPayload := strings.Join([]string{"1", "abcdefghijklmnop", "ffk", "ffk", "1", "z,10"}, ".")
	legacyValue := legacyPayload + "." + browserHistorySignature(legacyPayload, secret)
	state, ok = parseBrowserHistoryCookie(legacyValue, secret)
	require.True(t, ok)
	require.Equal(t, []int64{35, 36}, state.userIDs)
}

func TestParseBrowserHistoryCookieRejectsInvalidSignatureAndOversize(t *testing.T) {
	secret := "test-shared-cookie-secret-32bytes"
	valid := signBrowserHistoryCookie(browserHistoryCookieState{
		id:         "abcdefghijklmnop",
		firstDay:   20000,
		lastDay:    20001,
		visitCount: 5,
		userIDs:    []int64{1, 2, 3},
	}, secret)
	_, ok := parseBrowserHistoryCookie(valid, secret)
	require.True(t, ok)

	_, ok = parseBrowserHistoryCookie(valid[:len(valid)-1]+"x", secret)
	require.False(t, ok)

	_, ok = parseBrowserHistoryCookie(strings.Repeat("a", browserHistoryCookieMaxLength+1), secret)
	require.False(t, ok)
}

func formatBase36(value int64) string {
	return strconv.FormatInt(value, 36)
}
