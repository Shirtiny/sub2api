package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
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
	public := httptest.NewRecorder()
	r.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/api/v1/waitlist", nil))
	require.Equal(t, http.StatusNotFound, public.Code)
}
