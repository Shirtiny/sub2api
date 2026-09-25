//go:build unit

package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestPresaleRefundRequiresReasonBeforeChangingEntitlements(t *testing.T) {
	ctx := context.Background()
	s, o, sub := activePresaleRefundFixture(t, 1)
	amount := presaleTestRefundAmount(t, s, o.ID)
	auditsBefore := s.entClient.PaymentAuditLog.Query().CountX(ctx)
	for _, tc := range []struct{ reason, code string }{
		{"", "PRESALE_REFUND_REASON_REQUIRED"},
		{" \t\n\u3000", "PRESALE_REFUND_REASON_REQUIRED"},
		{strings.Repeat("退", 501), "PRESALE_REFUND_REASON_TOO_LONG"},
		{strings.Repeat("🌙", 501), "PRESALE_REFUND_REASON_TOO_LONG"},
	} {
		err := s.RequestRefund(ctx, o.ID, o.UserID, tc.reason, amount)
		require.Equal(t, tc.code, infraerrors.Reason(err))
		current := s.entClient.PaymentOrder.GetX(ctx, o.ID)
		require.Equal(t, OrderStatusCompleted, current.Status)
		require.Nil(t, current.RefundRequestedAt)
		require.Nil(t, current.RefundRequestReason)
		after := s.entClient.UserSubscription.GetX(ctx, sub.ID)
		require.Equal(t, sub.Status, after.Status)
		require.True(t, sub.ExpiresAt.Equal(after.ExpiresAt))
		require.Equal(t, auditsBefore, s.entClient.PaymentAuditLog.Query().CountX(ctx))
	}
	// Persist the customer's actual text, including Unicode, rather than a fixed UI label.
	reason := strings.Repeat("🌙", 500)
	require.NoError(t, s.RequestRefund(ctx, o.ID, o.UserID, " \n"+reason+"\t ", amount))
	accepted := s.entClient.PaymentOrder.GetX(ctx, o.ID)
	require.Equal(t, reason, *accepted.RefundRequestReason)
	require.Equal(t, OrderStatusRefundRequested, accepted.Status)
	require.NoError(t, s.RequestRefund(ctx, o.ID, o.UserID, "retry", amount))
	require.Equal(t, reason, *s.entClient.PaymentOrder.GetX(ctx, o.ID).RefundRequestReason)
}

func TestPresaleRefundQuoteCouponNoticeUsesOrderSnapshot(t *testing.T) {
	now := time.Now()
	start, end := now.Add(7*24*time.Hour), now.Add(38*24*time.Hour)
	for _, tc := range []struct {
		code     string
		discount float64
		applied  bool
	}{{"", 0, false}, {"CAFE-PUBLIC-40OFF", 40, true}, {"CAFE-MEMBER", 0, true}, {"", 40, true}} {
		o := &dbent.PaymentOrder{Status: OrderStatusCompleted, Amount: 100, PayAmount: 60, PresaleStartsAt: &start, PresaleExpiresAt: &end, CafeCouponCode: &tc.code, CafeCouponDiscount: tc.discount}
		quote, err := PresaleRefundQuoteForOrder(o, now)
		require.NoError(t, err)
		require.Equal(t, tc.applied, quote.CouponApplied)
		require.Equal(t, 60.0, quote.GatewayAmount, "notice must not affect refund money")
	}
}

func TestPresaleRefundLegacyQuoteKeepsMoneyAndAddsCouponNotice(t *testing.T) {
	ctx := context.Background()
	s, o, _ := activePresaleRefundFixture(t, 1)
	o = s.entClient.PaymentOrder.UpdateOneID(o.ID).SetCafeCouponCode("CAFE-PUBLIC-40OFF").SaveX(ctx)
	amount := presaleTestRefundAmount(t, s, o.ID)
	require.NoError(t, s.RequestRefund(ctx, o.ID, o.UserID, "changed plans", amount))
	audit := s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("PRESALE_REFUND_REQUESTED")).OnlyX(ctx)
	var detail map[string]any
	require.NoError(t, json.Unmarshal([]byte(audit.Detail), &detail))
	delete(detail["quote"].(map[string]any), "coupon_applied")
	legacy, err := json.Marshal(detail)
	require.NoError(t, err)
	s.entClient.PaymentAuditLog.UpdateOneID(audit.ID).SetDetail(string(legacy)).ExecX(ctx)
	quote, err := s.GetPresaleRefundQuote(ctx, s.entClient.PaymentOrder.GetX(ctx, o.ID), time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.True(t, quote.CouponApplied)
	require.Equal(t, amount, quote.GatewayAmount)
}
