package routes

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminPromoCafeCouponRoutesPrecedePromoID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/admin")
	h := &handler.Handlers{Admin: &handler.AdminHandlers{Promo: &admin.PromoHandler{}}}
	registerPromoCodeRoutes(group, h)

	routes := map[string]struct{}{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range []string{
		http.MethodGet + " /api/v1/admin/promo-codes/cafe-coupons",
		http.MethodGet + " /api/v1/admin/promo-codes/cafe-coupons/:id",
		http.MethodPatch + " /api/v1/admin/promo-codes/cafe-coupons/:id/status",
		http.MethodPost + " /api/v1/admin/promo-codes/cafe-coupons/:id/reset-claim-period",
		http.MethodPost + " /api/v1/admin/promo-codes/cafe-coupons/:id/void",
	} {
		_, ok := routes[route]
		require.True(t, ok, "route %s should be registered", route)
	}
}
