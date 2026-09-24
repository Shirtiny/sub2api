package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/shopspring/decimal"
)

// Called with the user row locked, in the transaction that changes REFUNDING to
// its terminal state. A failed gateway or repeated finalization cannot deduct
// twice. Paid-but-unreserved failures never credited points in the first place.
func (s *PaymentService) refundPresaleMembershipTx(ctx context.Context, tx *dbent.Tx, p *RefundPlan) error {
	grant, err := tx.PaymentAuditLog.Query().Where(
		paymentauditlog.OrderIDEQ(strconv.FormatInt(p.OrderID, 10)),
		paymentauditlog.ActionEQ("PRESALE_RESERVED"),
	).Only(ctx)
	if dbent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load presale membership grant: %w", err)
	}
	var detail struct {
		MembershipPoints *float64 `json:"membership_points"`
	}
	if err := json.Unmarshal([]byte(grant.Detail), &detail); err != nil {
		return fmt.Errorf("decode presale membership grant: %w", err)
	}
	// Before the explicit snapshot, every PRESALE_RESERVED audit was committed
	// atomically with AddTotalRecharged(order.PayAmount).
	credited := p.Order.PayAmount
	if detail.MembershipPoints != nil {
		credited = *detail.MembershipPoints
	}
	if credited < 0 || math.IsNaN(credited) || math.IsInf(credited, 0) || p.GatewayAmount < 0 || math.IsNaN(p.GatewayAmount) || math.IsInf(p.GatewayAmount, 0) {
		return fmt.Errorf("invalid presale membership refund amount")
	}
	u, err := tx.User.Get(ctx, p.Order.UserID)
	if err != nil {
		return fmt.Errorf("load presale membership balance: %w", err)
	}
	// Points were earned on actual payment, not nominal plan/coupon value.
	// Reverse the money returned; retained fees/used service keep their points.
	wanted := decimal.Min(decimal.NewFromFloat(credited), decimal.NewFromFloat(p.GatewayAmount))
	available := decimal.Max(decimal.Zero, decimal.NewFromFloat(u.TotalRecharged))
	deducted := decimal.Min(wanted, available)
	if err := tx.User.UpdateOneID(u.ID).SetTotalRecharged(available.Sub(deducted).InexactFloat64()).Exec(ctx); err != nil {
		return fmt.Errorf("refund presale membership points: %w", err)
	}
	return s.writeAuditLogStrict(ctx, p.OrderID, "PRESALE_MEMBERSHIP_REFUNDED", "admin", map[string]any{
		"credited": credited, "gateway_amount": p.GatewayAmount,
		"deducted": deducted.InexactFloat64(), "remaining": available.Sub(deducted).InexactFloat64(),
	})
}
