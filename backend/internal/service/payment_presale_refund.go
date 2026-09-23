package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionconcurrencyentitlement"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionearlyresetentitlement"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

type PresaleRefundQuote struct {
	RefundAmount  float64 `json:"refund_amount"`  // order accounting amount
	GatewayAmount float64 `json:"gateway_amount"` // actual money returned
	FeePercent    int     `json:"fee_percent"`
	UnusedDays    int     `json:"unused_days"`
	Policy        string  `json:"policy"`
	Currency      string  `json:"currency"`
}

func PresaleRefundQuoteForOrder(o *dbent.PaymentOrder, now time.Time) (*PresaleRefundQuote, error) {
	if o == nil || o.PresaleStartsAt == nil || o.PresaleExpiresAt == nil {
		return nil, infraerrors.BadRequest("NOT_PRESALE", "not a presale order")
	}
	allowedStatus := psSliceContains([]string{OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefundFailed}, o.Status) || (o.Status == OrderStatusFailed && o.PaidAt != nil)
	if !allowedStatus {
		return nil, infraerrors.BadRequest("INVALID_STATUS", "order does not allow a refund request")
	}
	if o.RefundRequestedAt != nil {
		now = *o.RefundRequestedAt
	}
	start, end := *o.PresaleStartsAt, *o.PresaleExpiresAt
	days := int(end.Sub(start).Hours() / 24)
	if days <= 0 {
		return nil, fmt.Errorf("invalid presale period")
	}
	q := &PresaleRefundQuote{Policy: "full", Currency: PaymentOrderCurrency(o), UnusedDays: days}
	ratio := 1.0
	switch {
	// A paid order never activated because of a service failure must remain
	// fully refundable, including after the advertised term has ended.
	case o.PresaleActivatedAt == nil && (!now.Before(start) || o.FailedAt != nil || o.Status == OrderStatusFailed):
		q.Policy = "unfulfilled"
	case now.Before(start.Add(-72 * time.Hour)):
	case now.Before(start):
		ratio, q.FeePercent, q.Policy = .8, 20, "preparation"
	case !now.Before(end.Add(-7 * 24 * time.Hour)):
		return nil, infraerrors.BadRequest("PRESALE_REFUND_LAST_WEEK", "daily refunds are unavailable during the final seven days")
	default:
		q.UnusedDays = int(math.Floor(end.Sub(now).Hours() / 24))
		ratio, q.Policy = float64(q.UnusedDays)/float64(days), "unused_days"
	}
	q.RefundAmount = math.Round(o.Amount*ratio*100) / 100
	q.GatewayAmount = decimal.NewFromFloat(o.PayAmount).Mul(decimal.NewFromFloat(ratio)).Round(int32(payment.CurrencyMaxFractionDigits(q.Currency))).InexactFloat64()
	return q, nil
}

// User requests freeze the policy timestamp and stop only this order's term in
// the same transaction. Provider execution stays in the existing admin flow.
func (s *PaymentService) requestPresaleRefund(ctx context.Context, o *dbent.PaymentOrder, uid int64, reason string, operator string, expectedGatewayAmount float64) error {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := lockPaymentUserForUpdate(txCtx, tx, uid); err != nil {
		return err
	}
	query := tx.PaymentOrder.Query().Where(paymentorder.IDEQ(o.ID), paymentorder.UserIDEQ(uid))
	if supportsForUpdate(tx.Client()) {
		query = query.ForUpdate()
	}
	current, err := query.Only(txCtx)
	if err != nil {
		return err
	}
	if current.Status == OrderStatusRefundRequested {
		return nil
	}
	allowedStatus := current.Status == OrderStatusCompleted || (current.Status == OrderStatusFailed && current.PaidAt != nil)
	if !allowedStatus {
		return infraerrors.Conflict("CONFLICT", "presale order status changed")
	}
	now := time.Now()
	quote, err := PresaleRefundQuoteForOrder(current, now)
	if err != nil {
		return err
	}
	if math.Abs(quote.GatewayAmount-expectedGatewayAmount) > .0001 {
		return infraerrors.Conflict("PRESALE_REFUND_AMOUNT_CHANGED", "refund amount changed; review the latest quote before cancelling")
	}
	if err := cancelPresaleEntitlementTx(txCtx, tx, current, now); err != nil {
		return err
	}
	if _, err := tx.PaymentOrder.UpdateOneID(current.ID).SetStatus(OrderStatusRefundRequested).SetRefundRequestedAt(now).SetRefundRequestedBy(fmt.Sprint(uid)).SetRefundRequestReason(strings.TrimSpace(reason)).SetRefundAmount(quote.RefundAmount).Save(txCtx); err != nil {
		return err
	}
	if err := s.writeAuditLogStrict(txCtx, current.ID, "PRESALE_REFUND_REQUESTED", operator, map[string]any{"policy": quote.Policy, "amount": quote.RefundAmount, "gateway_amount": quote.GatewayAmount, "at": now}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if s.subscriptionSvc != nil && current.SubscriptionGroupID != nil {
		s.subscriptionSvc.invalidateSubscriptionCaches(ctx, uid, *current.SubscriptionGroupID)
	}
	return nil
}

func cancelPresaleEntitlementTx(ctx context.Context, tx *dbent.Tx, o *dbent.PaymentOrder, now time.Time) error {
	if o.PresaleActivatedAt == nil {
		return nil
	} // never deduct an unrelated current subscription
	if o.PresaleSubscriptionID == nil || o.PresaleExpiresAt == nil {
		return fmt.Errorf("missing presale entitlement")
	}
	query := tx.UserSubscription.Query().Where(usersubscription.IDEQ(*o.PresaleSubscriptionID), usersubscription.UserIDEQ(o.UserID), usersubscription.DeletedAtIsNil())
	if supportsForUpdate(tx.Client()) {
		query = query.ForUpdate()
	}
	sub, err := query.Only(ctx)
	if err != nil {
		return err
	}
	if !sub.ExpiresAt.Equal(*o.PresaleExpiresAt) || !sub.StartsAt.Equal(*o.PresaleStartsAt) {
		return infraerrors.Conflict("PRESALE_ENTITLEMENT_CHANGED", "subscription term changed; contact support for a reconciled refund")
	}
	// The grant may have been capped at 1000. Revoke the actual credited
	// resets, not the larger advertised snapshot, to preserve prior grants.
	audit, err := tx.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(o.ID, 10)), paymentauditlog.ActionEQ("PRESALE_ACTIVATED")).Only(ctx)
	if err != nil {
		return fmt.Errorf("load presale activation grant: %w", err)
	}
	var grant struct {
		ResetCards int `json:"reset_cards"`
	}
	if err := json.Unmarshal([]byte(audit.Detail), &grant); err != nil {
		return fmt.Errorf("decode presale activation grant: %w", err)
	}
	if grant.ResetCards < 0 || grant.ResetCards > o.PresaleResetCards {
		return fmt.Errorf("invalid presale reset grant")
	}
	update := tx.UserSubscription.UpdateOneID(sub.ID).SetExpiresAt(now).SetStatus(SubscriptionStatusExpired).SetResetCount(max(0, sub.ResetCount-grant.ResetCards))
	if sub.CustomExpiresAt != nil {
		update.SetCustomExpiresAt(now)
	}
	if _, err := update.Save(ctx); err != nil {
		return err
	}
	if _, err := tx.SubscriptionConcurrencyEntitlement.Update().Where(subscriptionconcurrencyentitlement.SourceOrderIDEQ(o.ID)).SetExpiresAt(now).Save(ctx); err != nil {
		return err
	}
	_, err = tx.SubscriptionEarlyResetEntitlement.Update().Where(subscriptionearlyresetentitlement.SourceOrderIDEQ(o.ID)).SetExpiresAt(now).Save(ctx)
	return err
}

func (s *PaymentService) preparePresaleRefund(ctx context.Context, o *dbent.PaymentOrder, amount float64, reason string) (*RefundPlan, error) {
	quote, err := PresaleRefundQuoteForOrder(o, time.Now())
	if err != nil {
		return nil, err
	}
	if amount > 0 && math.Abs(amount-quote.RefundAmount) > .009 {
		return nil, infraerrors.BadRequest("PRESALE_REFUND_AMOUNT_CHANGED", "use the refund amount calculated by the presale policy")
	}
	if strings.TrimSpace(reason) == "" {
		reason = psStringValue(o.RefundRequestReason)
	}
	return &RefundPlan{OrderID: o.ID, Order: o, RefundAmount: quote.RefundAmount, GatewayAmount: quote.GatewayAmount, Reason: reason, DeductionType: payment.DeductionTypeNone}, nil
}

func (s *PaymentService) claimPresaleRefund(ctx context.Context, p *RefundPlan) error {
	// An admin may initiate a refund without a preceding user request. Freeze
	// and cancel first; if the provider fails the request remains retryable.
	if p.Order.RefundRequestedAt == nil {
		if err := s.requestPresaleRefund(ctx, p.Order, p.Order.UserID, p.Reason, "admin", p.GatewayAmount); err != nil {
			return err
		}
	}
	current, err := s.entClient.PaymentOrder.Get(ctx, p.OrderID)
	if err != nil {
		return err
	}
	quote, err := PresaleRefundQuoteForOrder(current, time.Now())
	if err != nil {
		return err
	}
	if math.Abs(p.GatewayAmount-quote.GatewayAmount) > paymentAmountToleranceForCurrency(quote.Currency) {
		return infraerrors.Conflict("PRESALE_REFUND_AMOUNT_CHANGED", "refund window changed; refresh the order")
	}
	p.Order = current
	p.RefundAmount, p.GatewayAmount = quote.RefundAmount, quote.GatewayAmount
	return nil
}
