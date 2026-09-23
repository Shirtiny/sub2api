package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWaitlistListRequiresAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterAdminRoutes(r.Group("/api/v1"), &handler.Handlers{Admin: &handler.AdminHandlers{}, Waitlist: &handler.WaitlistHandler{}}, middleware.NewAdminAuthMiddleware(nil, nil, nil))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/waitlist", nil))
	require.Equal(t, http.StatusUnauthorized, w.Code)
	post := httptest.NewRecorder()
	r.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/v1/admin/waitlist/1/approve", nil))
	require.Equal(t, http.StatusUnauthorized, post.Code)
	publicPost := httptest.NewRecorder()
	r.ServeHTTP(publicPost, httptest.NewRequest(http.MethodPost, "/api/v1/waitlist/1/approve", nil))
	require.Equal(t, http.StatusNotFound, publicPost.Code)
	public := httptest.NewRecorder()
	r.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/api/v1/waitlist", nil))
	require.Equal(t, http.StatusNotFound, public.Code)
}

type waitlistRouteUserRepo struct {
	service.UserRepository
	current *service.User
}

func (r waitlistRouteUserRepo) GetByID(context.Context, int64) (*service.User, error) {
	return r.current, nil
}

func (r waitlistRouteUserRepo) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	return nil, nil
}

func TestWaitlistApprovalRejectsOrdinaryUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "waitlist-route-test", ExpireHour: 1}}
	auth := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	user := &service.User{ID: 12, Email: "ordinary@example.com", Status: service.StatusActive, Role: service.RoleUser, TokenVersion: 1, TokenVersionResolved: true}
	users := service.NewUserService(waitlistRouteUserRepo{current: user}, nil, nil, nil)
	token, err := auth.GenerateToken(user)
	require.NoError(t, err)
	r := gin.New()
	RegisterAdminRoutes(r.Group("/api/v1"), &handler.Handlers{Admin: &handler.AdminHandlers{}, Waitlist: &handler.WaitlistHandler{}}, middleware.NewAdminAuthMiddleware(auth, users, nil))
	for _, target := range []struct{ method, path string }{{http.MethodGet, "/api/v1/admin/waitlist"}, {http.MethodPost, "/api/v1/admin/waitlist/1/approve"}} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(target.method, target.path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusForbidden, w.Code)
	}
}
