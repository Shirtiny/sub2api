//go:build unit

package handler

import (
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPresaleOrderResponseAndResumeContracts(t *testing.T) {
	period := service.NextPresalePeriod(time.Now())
	o := &dbent.PaymentOrder{ID: 7, PresaleStartsAt: &period.StartsAt, PresaleExpiresAt: &period.ExpiresAt, PresalePlanName: "Astra", PresaleResetCards: 2, PresaleRenewal: true}
	user := sanitizePaymentOrderForResponse(o)
	public := buildPublicOrderResult(o)
	require.Equal(t, user.PresaleOrderInfo, public.PresaleOrderInfo)
	require.Equal(t, "Astra", user.PresalePlanName)
	require.True(t, user.PresaleRenewal)
	require.Nil(t, user.PresaleActivatedAt)
	req := CreateOrderRequest{PresaleMonth: "2099-12"}
	err := applyWeChatPaymentResumeClaims(&req, &service.WeChatPaymentResumeClaims{OpenID: "fixture", UserID: 91, OrderType: "subscription", PlanID: 7, PresaleMonth: period.Month}, 91)
	require.NoError(t, err)
	require.Equal(t, period.Month, req.PresaleMonth, "signed month overrides untrusted posted month")
	key := paymentCreateOrderIdempotencyPayload(service.CreateOrderRequest{PresaleMonth: period.Month, ExpectedPresaleBonusVersion: "gift-version"})
	require.Equal(t, period.Month, key.PresaleMonth, "month must be part of idempotency identity")
	require.Equal(t, "gift-version", key.ExpectedPresaleBonusVersion, "gift version must be part of idempotency identity")
}
