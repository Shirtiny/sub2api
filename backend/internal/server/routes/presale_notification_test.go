package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPresaleNoticeRequiresAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "presale-notice-route-test", ExpireHour: 1}}
	auth := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	user := &service.User{ID: 12, Email: "ordinary@example.com", Status: service.StatusActive, Role: service.RoleUser, TokenVersion: 1, TokenVersionResolved: true}
	users := service.NewUserService(waitlistRouteUserRepo{current: user}, nil, nil, nil)
	token, err := auth.GenerateToken(user)
	require.NoError(t, err)
	r := gin.New()
	RegisterPaymentRoutes(r.Group("/api/v1"), &handler.PaymentHandler{}, &handler.PaymentWebhookHandler{}, &admin.PaymentHandler{}, middleware.NewJWTAuthMiddleware(auth, users), middleware.NewAdminAuthMiddleware(auth, users, nil), nil)
	for _, endpoint := range []struct{ method, path string }{{"GET", "/presale-notice"}, {"POST", "/presale-notice/test"}, {"POST", "/presale-notice/send-next"}} {
		for _, bearer := range []string{"", token} {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(endpoint.method, "/api/v1/admin/payment"+endpoint.path, nil)
			if bearer != "" {
				req.Header.Set("Authorization", "Bearer "+bearer)
			}
			r.ServeHTTP(w, req)
			if bearer == "" {
				require.Equal(t, http.StatusUnauthorized, w.Code)
			} else {
				require.Equal(t, http.StatusForbidden, w.Code)
			}
		}
	}
}
