package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

type PresaleOfflineRequest struct {
	Mode              string    `json:"mode"`
	Amount            float64   `json:"amount"`
	Reason            string    `json:"reason"`
	Reference         string    `json:"reference"`
	Confirmed         bool      `json:"confirmed"`
	ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
}

type PresaleOfflineResult struct {
	Status           string `json:"status"`
	AffiliatePending bool   `json:"affiliate_pending"`
}

// Admin-only accounting of an external action. Never resolves a provider or
// calls a gateway. User/order/subscription locks match activation and refunds.
func (s *PaymentService) ProcessPresaleOffline(ctx context.Context, orderID, adminID int64, req PresaleOfflineRequest) (*PresaleOfflineResult, error) {
	req.Reason, req.Reference = strings.TrimSpace(req.Reason), strings.TrimSpace(req.Reference)
	if adminID <= 0 {
		return nil, infraerrors.Forbidden("FORBIDDEN", "an authenticated administrator is required")
	}
	if !req.Confirmed || req.ExpectedUpdatedAt.IsZero() || req.Reason == "" || utf8.RuneCountInString(req.Reason) > 500 || utf8.RuneCountInString(req.Reference) > 200 ||
		(req.Mode != "cancel" && req.Mode != "refund") || math.IsNaN(req.Amount) || math.IsInf(req.Amount, 0) ||
		(req.Mode == "cancel" && (req.Amount != 0 || req.Reference != "")) || (req.Mode == "refund" && (req.Amount <= 0 || req.Reference == "")) {
		return nil, infraerrors.BadRequest("PRESALE_OFFLINE_INVALID", "confirm the action and supply a valid reason, amount and offline reference")
	}
	before, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := lockPaymentUserForUpdate(txCtx, tx, before.UserID); err != nil {
		return nil, err
	}
	query := tx.PaymentOrder.Query().Where(paymentorder.IDEQ(orderID))
	if supportsForUpdate(tx.Client()) {
		query = query.ForUpdate()
	}
	o, err := query.Only(txCtx)
	if err != nil {
		return nil, err
	}
	if o.OrderType != payment.OrderTypeSubscription || o.PresaleStartsAt == nil || o.PresaleExpiresAt == nil || o.PaidAt == nil || !isValidProviderAmount(o.PayAmount) || !isValidProviderAmount(o.Amount) {
		return nil, infraerrors.BadRequest("PRESALE_OFFLINE_NOT_PAID", "only paid presale subscriptions support offline handling")
	}
	paid, amount := decimal.NewFromFloat(o.PayAmount), decimal.NewFromFloat(req.Amount)
	if amount.GreaterThan(paid) || !amount.Equal(amount.Round(int32(payment.CurrencyMaxFractionDigits(PaymentOrderCurrency(o))))) {
		return nil, infraerrors.BadRequest("PRESALE_OFFLINE_AMOUNT", "offline amount must fit the payment currency and not exceed the amount paid")
	}
	action := "PRESALE_CANCELLED"
	if req.Mode == "refund" {
		action = "PRESALE_OFFLINE_REFUND"
	}
	// Retrying an identical completed operation is safe even with a stale form.
	// A different amount/reference must never become a second refund.
	replay := (req.Mode == "cancel" && o.Status == OrderStatusPresaleCancelled) ||
		(req.Mode == "refund" && psSliceContains([]string{OrderStatusRefunded, OrderStatusPartiallyRefunded}, o.Status))
	if replay {
		audit, err := tx.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(o.ID, 10)), paymentauditlog.ActionEQ(action)).Only(txCtx)
		var recorded PresaleOfflineRequest
		if err != nil || json.Unmarshal([]byte(audit.Detail), &recorded) != nil || recorded.Mode != req.Mode || recorded.Amount != req.Amount || recorded.Reason != req.Reason || recorded.Reference != req.Reference {
			return nil, infraerrors.Conflict("PRESALE_OFFLINE_STATE", "the order has already been handled differently")
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return s.finishPresaleOffline(ctx, o, req.Mode), nil
	}
	if !o.UpdatedAt.Equal(req.ExpectedUpdatedAt) {
		return nil, infraerrors.Conflict("PRESALE_OFFLINE_STALE", "order changed; reload and review before proceeding")
	}
	allowed := o.Status == OrderStatusCompleted || (o.Status == OrderStatusFailed && o.PaidAt != nil) ||
		(req.Mode == "refund" && (o.Status == OrderStatusRefundRequested || o.Status == OrderStatusPresaleCancelled))
	if !allowed || o.RefundAt != nil {
		return nil, infraerrors.Conflict("PRESALE_OFFLINE_STATE", "order status does not allow this offline action")
	}
	onlineAttempt, err := tx.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(o.ID, 10)), paymentauditlog.ActionIn(
		"PRESALE_ONLINE_REFUND_STARTED", "REFUND_SUCCESS", "REFUND_FAILED", "REFUND_GATEWAY_FAILED", "REFUND_ROLLBACK_FAILED", "DEV_PAYMENT_REFUND_SUCCESS",
	)).Exist(txCtx)
	if err != nil {
		return nil, err
	}
	if onlineAttempt {
		return nil, infraerrors.Conflict("PRESALE_OFFLINE_ONLINE_REFUND", "an online refund was attempted; reconcile the provider before any offline settlement")
	}
	now := time.Now()
	if o.Status == OrderStatusPresaleCancelled || o.RefundRequestedAt != nil {
		// These paths already cancelled this exact term. Never deduct it again,
		// especially if the shared subscription row now belongs to a later term.
		cancelAction := "PRESALE_REFUND_REQUESTED"
		if o.Status == OrderStatusPresaleCancelled {
			cancelAction = "PRESALE_CANCELLED"
		}
		exists, err := tx.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(o.ID, 10)), paymentauditlog.ActionEQ(cancelAction)).Exist(txCtx)
		if err != nil || !exists {
			return nil, infraerrors.Conflict("PRESALE_OFFLINE_STATE", "missing previous entitlement cancellation audit")
		}
	} else if err := cancelPresaleEntitlementTx(txCtx, tx, o, now); err != nil {
		return nil, err
	}
	status := OrderStatusPresaleCancelled
	accounting := 0.0
	update := tx.PaymentOrder.UpdateOneID(o.ID)
	if req.Mode == "refund" {
		status = OrderStatusPartiallyRefunded
		if amount.Equal(paid) {
			status = OrderStatusRefunded
		}
		accounting = decimal.NewFromFloat(o.Amount).Mul(amount).Div(paid).Round(2).InexactFloat64()
		update.SetRefundAmount(accounting).SetRefundReason(req.Reason).SetRefundAt(now)
		if err := s.refundPresaleMembershipTx(txCtx, tx, &RefundPlan{OrderID: o.ID, Order: o, GatewayAmount: req.Amount}); err != nil {
			return nil, err
		}
	}
	updated, err := update.SetStatus(status).Save(txCtx)
	if err != nil {
		return nil, err
	}
	if err := s.writeAuditLogStrict(txCtx, o.ID, action, fmt.Sprintf("admin:%d", adminID), map[string]any{
		"mode": req.Mode, "amount": req.Amount, "accounting_amount": accounting, "currency": PaymentOrderCurrency(o),
		"reason": req.Reason, "reference": req.Reference, "previous_status": o.Status, "admin_id": adminID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.finishPresaleOffline(ctx, updated, req.Mode), nil
}

func (s *PaymentService) finishPresaleOffline(ctx context.Context, o *dbent.PaymentOrder, mode string) *PresaleOfflineResult {
	if s.subscriptionSvc != nil && o.SubscriptionGroupID != nil {
		s.subscriptionSvc.invalidateSubscriptionCaches(ctx, o.UserID, *o.SubscriptionGroupID)
	}
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, o.UserID)
	}
	out := &PresaleOfflineResult{Status: o.Status}
	if mode == "refund" && s.affiliateService != nil {
		// The existing clawback is idempotent. An identical retry can complete
		// this secondary bookkeeping without repeating the refund or deduction.
		if _, err := s.affiliateService.ClawbackInviteRebateForRefund(ctx, o.ID, o.RefundAmount, o.Amount); err != nil {
			out.AffiliatePending = true
			s.writeAuditLog(ctx, o.ID, "AFFILIATE_REBATE_CLAWBACK_FAILED", "admin", map[string]any{"mode": "offline", "error": err.Error()})
		}
	}
	return out
}
