package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestPromoHandlerCafeCouponAdminValidationAndErrors(t *testing.T) {
	client := newPromoCafeCouponTestClient(t)
	router := newPromoCafeCouponRouter(service.NewPaymentService(client, nil, nil, nil, nil, nil, nil, nil, nil))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/promo-codes/cafe-coupons?membership_level=4", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/promo-codes/cafe-coupons?status=bad", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "CAFE_COUPON_STATUS_INVALID", responseReason(t, rec.Body.Bytes()))

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/promo-codes/cafe-coupons?type=bonus", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "CAFE_COUPON_TYPE_INVALID", responseReason(t, rec.Body.Bytes()))

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/promo-codes/cafe-coupons/999", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "CAFE_COUPON_NOT_FOUND", responseReason(t, rec.Body.Bytes()))
}

func TestPromoHandlerCafeCouponPaidRestoreConflictAndReset(t *testing.T) {
	ctx := context.Background()
	client := newPromoCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("admin-handler-cafe@example.com").SetPasswordHash("hash").SetUsername("admin-handler-cafe").Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).SetAmount(100).SetPayAmount(90).SetFeeRate(0).SetRechargeCode("PAY-HANDLER-CAFE").SetOutTradeNo("PAY-HANDLER-CAFE").
		SetPaymentType("alipay").SetPaymentTradeNo("TRADE-HANDLER-CAFE").SetOrderType("balance").SetStatus(service.OrderStatusCompleted).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("app.example.com").
		Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	resetCoupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-HANDLER-RESET").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(service.CafeCouponTypeCash).SetValue(10).
		SetPeriod(service.CafeCouponPeriodMonth).SetPeriodStart(now).SetPeriodEnd(now.AddDate(0, 1, 0)).SetStatus(service.CafeCouponStatusIssued).
		Save(ctx)
	require.NoError(t, err)
	paidCoupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-HANDLER-PAID").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(service.CafeCouponTypeCash).SetValue(10).
		SetPeriod(service.CafeCouponPeriodMonth).SetPeriodStart(now.Add(time.Minute)).SetPeriodEnd(now.AddDate(0, 1, 0)).SetStatus(service.CafeCouponStatusApplied).SetOrderID(order.ID).SetAppliedAt(now).SetCreatedAt(now.Add(time.Minute)).
		Save(ctx)
	require.NoError(t, err)
	router := newPromoCafeCouponRouter(service.NewPaymentService(client, nil, nil, nil, nil, nil, nil, nil, nil))

	body := bytes.NewBufferString(`{"status":"issued"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/admin/promo-codes/cafe-coupons/%d/status", paidCoupon.ID), body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var statusOut struct {
		Data struct {
			Status    string     `json:"status"`
			OrderID   *int64     `json:"order_id"`
			AppliedAt *time.Time `json:"applied_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &statusOut))
	require.Equal(t, service.CafeCouponStatusIssued, statusOut.Data.Status)
	require.Nil(t, statusOut.Data.OrderID)
	require.Nil(t, statusOut.Data.AppliedAt)
	storedPaid, err := client.CafeCoupon.Get(ctx, paidCoupon.ID)
	require.NoError(t, err)
	require.Equal(t, service.CafeCouponStatusIssued, storedPaid.Status)
	require.Nil(t, storedPaid.OrderID)
	require.Nil(t, storedPaid.AppliedAt)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/admin/promo-codes/cafe-coupons/%d/reset-claim-period", resetCoupon.ID), nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var resetOut struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resetOut))
	storedReset, err := client.CafeCoupon.Get(ctx, resetCoupon.ID)
	require.NoError(t, err)
	storedPaid, err = client.CafeCoupon.Get(ctx, paidCoupon.ID)
	require.NoError(t, err)
	mutatedID := resetCoupon.ID
	if !storedPaid.PeriodStart.Equal(paidCoupon.PeriodStart) || !storedPaid.CreatedAt.Equal(paidCoupon.CreatedAt) {
		mutatedID = paidCoupon.ID
	}
	require.Equal(t, mutatedID, resetOut.Data.ID)
	require.Equal(t, service.CafeCouponStatusIssued, storedPaid.Status)
	if mutatedID == resetCoupon.ID {
		require.Equal(t, service.CafeCouponStatusVoid, storedReset.Status)
	}
}

func newPromoCafeCouponRouter(paymentService *service.PaymentService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewPromoHandler(nil, paymentService)
	router.GET("/admin/promo-codes/cafe-coupons", h.ListCafeCoupons)
	router.GET("/admin/promo-codes/cafe-coupons/:id", h.GetCafeCoupon)
	router.PATCH("/admin/promo-codes/cafe-coupons/:id/status", h.UpdateCafeCouponStatus)
	router.POST("/admin/promo-codes/cafe-coupons/:id/reset-claim-period", h.ResetCafeCouponClaimPeriod)
	return router
}

func newPromoCafeCouponTestClient(t *testing.T) *dbent.Client {
	t.Helper()
	dbName := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name())
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func responseReason(t *testing.T, body []byte) string {
	t.Helper()
	var out struct {
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(body, &out))
	return out.Reason
}
