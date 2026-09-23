package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Payment completion allocates a dated pending entitlement in the order ledger,
// not live subscription quota. Snapshot fields survive plan edits/deletion.
func (s *PaymentService) completePresale(ctx context.Context, o *dbent.PaymentOrder, lease *paymentFulfillmentLease) error {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := lockPaymentUserForUpdate(txCtx, tx, o.UserID); err != nil {
		return err
	}
	if o.PresaleExpiresAt == nil || o.SubscriptionSourceGroupID == nil {
		return fmt.Errorf("incomplete presale snapshot")
	}
	if err := s.withEntClient(tx.Client()).checkPresaleSlot(txCtx, o.UserID, *o.SubscriptionSourceGroupID, *o.PresaleStartsAt, o.ID); err != nil {
		return err
	}
	query := tx.PaymentOrder.Query().Where(paymentorder.IDEQ(o.ID), paymentorder.StatusEQ(OrderStatusRecharging), paymentorder.UpdatedAtEQ(lease.version))
	if supportsForUpdate(tx.Client()) {
		query = query.ForUpdate()
	}
	if _, err := query.Only(txCtx); err != nil {
		return infraerrors.Conflict("CONFLICT", "presale fulfillment lease changed")
	}
	reserved, err := tx.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(o.ID, 10)), paymentauditlog.ActionEQ("PRESALE_RESERVED")).Exist(txCtx)
	if err != nil {
		return err
	}
	if !reserved {
		if _, err := tx.User.UpdateOneID(o.UserID).AddTotalRecharged(o.PayAmount).Save(txCtx); err != nil {
			return err
		}
		if err := s.writeAuditLogStrict(txCtx, o.ID, "PRESALE_RESERVED", "system", map[string]any{"starts_at": o.PresaleStartsAt, "expires_at": o.PresaleExpiresAt, "renewal": o.PresaleRenewal}); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := s.applyAffiliateRebateForOrder(ctx, o); err != nil {
		return err
	}
	return s.markCompleted(ctx, o, lease, "PRESALE_RESERVED")
}

// Uses the existing payment maintenance worker. Locks + the activation marker
// make retries and multiple workers safe without a separate scheduler/queue.
func (s *PaymentService) ActivateDuePresales(ctx context.Context, now time.Time) (int, error) {
	count := 0
	var cursor int64
	for {
		orders, err := s.entClient.PaymentOrder.Query().Where(paymentorder.IDGT(cursor), paymentorder.StatusEQ(OrderStatusCompleted), paymentorder.PresaleActivatedAtIsNil(), paymentorder.PresaleStartsAtLTE(now)).Order(paymentorder.ByID()).Limit(100).All(ctx)
		if err != nil {
			return count, err
		}
		if len(orders) == 0 {
			return count, nil
		}
		for _, o := range orders {
			cursor = o.ID
			activated, err := s.activatePresale(ctx, o.ID, now)
			if err != nil {
				slog.Error("presale activation failed", "order_id", o.ID, "error", err)
				continue
			}
			if activated {
				count++
			}
		}
	}
}

func (s *PaymentService) activatePresale(ctx context.Context, orderID int64, now time.Time) (bool, error) {
	before, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return false, err
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := lockPaymentUserForUpdate(txCtx, tx, before.UserID); err != nil {
		return false, err
	}
	query := tx.PaymentOrder.Query().Where(paymentorder.IDEQ(orderID))
	if supportsForUpdate(tx.Client()) {
		query = query.ForUpdate()
	}
	o, err := query.Only(txCtx)
	if err != nil {
		return false, err
	}
	if o.Status != OrderStatusCompleted || o.PresaleActivatedAt != nil || o.PresaleStartsAt == nil || now.Before(*o.PresaleStartsAt) {
		return false, nil
	}
	if o.PresaleExpiresAt == nil || o.SubscriptionGroupID == nil || o.PlanID == nil {
		return false, fmt.Errorf("invalid presale snapshot")
	}
	if !now.Before(*o.PresaleExpiresAt) {
		return false, fmt.Errorf("presale period ended without activation; manual refund required")
	}
	active, err := tx.User.Query().Where(user.IDEQ(o.UserID), user.StatusEQ(StatusActive), user.DeletedAtIsNil()).Exist(txCtx)
	if err != nil {
		return false, err
	}
	if !active {
		return false, fmt.Errorf("presale user is not active")
	}
	group, err := s.groupRepo.GetByID(txCtx, *o.SubscriptionGroupID)
	if err != nil {
		return false, err
	}
	if group.Status != StatusActive || !group.IsSubscriptionType() {
		return false, fmt.Errorf("presale group unavailable")
	}
	existingQuery := tx.UserSubscription.Query().Where(usersubscription.UserIDEQ(o.UserID), usersubscription.GroupIDEQ(*o.SubscriptionGroupID), usersubscription.DeletedAtIsNil())
	if supportsForUpdate(tx.Client()) {
		existingQuery = existingQuery.ForUpdate()
	}
	sub, err := existingQuery.Only(txCtx)
	if err != nil && !dbent.IsNotFound(err) {
		return false, err
	}
	if sub != nil && sub.ExpiresAt.After(*o.PresaleStartsAt) {
		return false, infraerrors.Conflict("PRESALE_COVERAGE_OVERLAP", "subscription was extended after purchase; manual reconciliation required")
	}
	if sub == nil {
		sub, err = tx.UserSubscription.Create().SetUserID(o.UserID).SetGroupID(*o.SubscriptionGroupID).SetStartsAt(*o.PresaleStartsAt).SetExpiresAt(*o.PresaleExpiresAt).SetStatus(SubscriptionStatusActive).Save(txCtx)
		if err != nil {
			return false, err
		}
	}
	// A new monthly term resets the old term's meters once, at activation only.
	resetCards := min(1000, sub.ResetCount+o.PresaleResetCards)
	update := tx.UserSubscription.UpdateOneID(sub.ID).
		SetStartsAt(*o.PresaleStartsAt).SetExpiresAt(*o.PresaleExpiresAt).SetStatus(SubscriptionStatusActive).
		SetDailyUsageUsd(0).SetWeeklyUsageUsd(0).SetMonthlyUsageUsd(0).
		ClearDailyWindowStart().ClearWeeklyWindowStart().ClearMonthlyWindowStart().
		SetResetCount(resetCards).SetNotes(appendSubscriptionNotes(psStringValue(sub.Notes), paymentSubscriptionOrderNote(o.ID))).
		SetEarlyResetEnabled(o.SubscriptionEarlyResetEnabled).SetEarlyResetDurationDays(o.SubscriptionEarlyResetDurationDays).
		ClearCustomMultiplier().ClearCustomSourcePlanID().ClearCustomSourceGroupID().ClearCustomExpiresAt().ClearCustomDisplayName()
	multiplier := 1
	if o.SubscriptionMultiplier != nil {
		multiplier = *o.SubscriptionMultiplier
	}
	if multiplier > 1 {
		update.SetCustomMultiplier(multiplier).SetCustomSourcePlanID(*o.PlanID).SetCustomSourceGroupID(*o.SubscriptionGroupID).SetCustomExpiresAt(*o.PresaleExpiresAt).SetCustomDisplayName(o.PresalePlanName)
	}
	if _, err := update.Save(txCtx); err != nil {
		return false, err
	}
	input := &AssignSubscriptionInput{UserID: o.UserID, GroupID: *o.SubscriptionGroupID, PlanConcurrency: o.SubscriptionConcurrency, PlanConcurrencySourceID: &o.ID, EarlyResetEnabled: &o.SubscriptionEarlyResetEnabled, EarlyResetDurationDays: &o.SubscriptionEarlyResetDurationDays, EarlyResetSourceOrderID: &o.ID}
	if err := s.subscriptionSvc.recordPlanConcurrencyEntitlement(txCtx, sub.ID, input, *o.PresaleStartsAt, *o.PresaleExpiresAt); err != nil {
		return false, err
	}
	if err := s.subscriptionSvc.recordEarlyResetEntitlement(txCtx, sub.ID, input, *o.PresaleStartsAt, *o.PresaleExpiresAt, multiplier > 1); err != nil {
		return false, err
	}
	if _, err := tx.PaymentOrder.UpdateOneID(o.ID).SetPresaleActivatedAt(now).SetPresaleSubscriptionID(sub.ID).Save(txCtx); err != nil {
		return false, err
	}
	if err := s.writeAuditLogStrict(txCtx, o.ID, "PRESALE_ACTIVATED", "system", map[string]any{"subscription_id": sub.ID, "starts_at": o.PresaleStartsAt, "expires_at": o.PresaleExpiresAt, "reset_cards": resetCards - sub.ResetCount}); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	s.subscriptionSvc.invalidateSubscriptionCaches(ctx, o.UserID, *o.SubscriptionGroupID)
	return true, nil
}
