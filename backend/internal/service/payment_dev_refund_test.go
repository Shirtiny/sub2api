//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/stretchr/testify/require"
)

func TestDevPaymentRefundRequiresLocalModeAndAuditedUnboundOrder(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	o := newPresaleOrder(t, s, u, p, NextPresalePeriod(time.Now()))
	o.PaymentTradeNo = "dev-auto-success-" + o.OutTradeNo
	t.Setenv(paymentDevAutoSuccessEnv, paymentDevAutoSuccessToken)
	t.Setenv(paymentDevEnvironmentEnv, "local")
	for _, key := range []string{"APP_ENV", "ENVIRONMENT", "NODE_ENV", "GO_ENV"} {
		t.Setenv(key, "")
	}
	require.False(t, s.isDevAutoSuccessOrder(ctx, o), "prefix without audit is insufficient")
	require.NoError(t, s.writeAuditLogStrict(ctx, o.ID, "DEV_PAYMENT_AUTO_SUCCESS", "test", map[string]any{}))
	require.True(t, s.isDevAutoSuccessOrder(ctx, o))
	for _, key := range []string{"APP_ENV", "ENVIRONMENT", "NODE_ENV", "GO_ENV"} {
		t.Run(key, func(t *testing.T) { t.Setenv(key, "production"); require.False(t, s.isDevAutoSuccessOrder(ctx, o)) })
	}
	t.Run("disabled", func(t *testing.T) {
		t.Setenv(paymentDevAutoSuccessEnv, "")
		require.False(t, s.isDevAutoSuccessOrder(ctx, o))
	})
	t.Run("wrong environment", func(t *testing.T) {
		t.Setenv(paymentDevEnvironmentEnv, "staging")
		require.False(t, s.isDevAutoSuccessOrder(ctx, o))
	})
	for _, change := range []func(*dbent.PaymentOrder){
		func(x *dbent.PaymentOrder) { x.PaymentTradeNo = "real-trade" },
		func(x *dbent.PaymentOrder) { x.PaidAt = nil },
		func(x *dbent.PaymentOrder) { v := "1"; x.ProviderInstanceID = &v },
		func(x *dbent.PaymentOrder) { v := "alipay"; x.ProviderKey = &v },
	} {
		copy := *o
		change(&copy)
		require.False(t, s.isDevAutoSuccessOrder(ctx, &copy))
	}
	require.False(t, s.isDevAutoSuccessOrder(ctx, nil))
}

func TestDevPresaleRefundUsesNormalLifecycleWithoutGateway(t *testing.T) {
	s, u, p := newPresaleFixture(t)
	ctx := context.Background()
	t.Setenv(paymentDevAutoSuccessEnv, paymentDevAutoSuccessToken)
	t.Setenv(paymentDevEnvironmentEnv, "local")
	for _, key := range []string{"APP_ENV", "ENVIRONMENT", "NODE_ENV", "GO_ENV"} {
		t.Setenv(key, "")
	}
	o := newPresaleOrder(t, s, u, p, NextPresalePeriod(time.Now().AddDate(0, 1, 0)))
	require.NoError(t, s.writeAuditLogStrict(ctx, o.ID, "DEV_PAYMENT_AUTO_SUCCESS", "test", map[string]any{}))
	s.entClient.PaymentOrder.UpdateOneID(o.ID).SetPaymentTradeNo("dev-auto-success-" + o.OutTradeNo).
		SetFeeRate(1).SetPayAmount(101).ExecX(ctx)
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	o = s.entClient.PaymentOrder.GetX(ctx, o.ID)
	quote, err := s.GetPresaleRefundQuote(ctx, o, time.Now())
	require.NoError(t, err)
	require.Equal(t, 100.0, quote.GatewayAmount)
	require.NoError(t, s.RequestRefund(ctx, o.ID, u.ID, "test refund", quote.GatewayAmount))
	plan, _, err := s.PrepareRefund(ctx, o.ID, quote.RefundAmount, "test refund", false, false)
	require.NoError(t, err)
	result, err := s.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.True(t, result.Success)
	refunded := s.entClient.PaymentOrder.GetX(ctx, o.ID)
	require.Equal(t, quote.RefundAmount, refunded.RefundAmount)
	require.Equal(t, OrderStatusPartiallyRefunded, refunded.Status, "the payment fee is retained while the term is fully cancelled")
	audit := s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(fmt.Sprint(o.ID)), paymentauditlog.ActionEQ("DEV_PAYMENT_REFUND_SUCCESS")).OnlyX(ctx)
	var detail struct {
		GatewayAmount float64 `json:"gatewayAmount"`
	}
	require.NoError(t, json.Unmarshal([]byte(audit.Detail), &detail))
	require.Equal(t, 100.0, detail.GatewayAmount)
	require.Zero(t, s.entClient.PaymentProviderInstance.Query().CountX(ctx))
	_, _, err = s.PrepareRefund(ctx, o.ID, quote.RefundAmount, "repeat", false, false)
	require.Error(t, err)
}
