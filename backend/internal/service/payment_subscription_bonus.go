package service

import (
	"context"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivity"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivityparticipation"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivityplan"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func resolveSubscriptionBonusForOrder(ctx context.Context, client *dbent.Client, userID, planID, expectedActivityID int64, now time.Time) (*SubscriptionBonusBenefit, error) {
	if client == nil || userID <= 0 || planID <= 0 {
		return nil, nil
	}
	activityQuery := client.PromotionActivity.Query().
		Where(
			promotionactivity.ActivityTypeEQ(PromotionActivityTypeSubscriptionBonusDays),
			promotionactivity.EnabledEQ(true),
			promotionactivity.StartsAtLTE(now),
			promotionactivity.EndsAtGT(now),
			promotionactivity.HasPlanBonusesWith(promotionactivityplan.PlanIDEQ(planID)),
		).
		WithPlanBonuses(func(q *dbent.PromotionActivityPlanQuery) {
			q.Where(promotionactivityplan.PlanIDEQ(planID))
		}).
		Order(dbent.Asc(promotionactivity.FieldEndsAt), dbent.Asc(promotionactivity.FieldID))
	if dbent.TxFromContext(ctx) != nil && supportsForUpdate(client) {
		// Orders only need a stable activity snapshot. A shared lock blocks
		// activity edits/deletion while allowing different users to create
		// orders for the same activity concurrently.
		activityQuery = activityQuery.ForShare()
	}
	activities, err := activityQuery.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve subscription bonus activity: %w", err)
	}
	for _, activity := range activities {
		if expectedActivityID > 0 && activity.ID != expectedActivityID {
			continue
		}
		used, err := client.PromotionActivityParticipation.Query().Where(
			promotionactivityparticipation.ActivityIDEQ(activity.ID),
			promotionactivityparticipation.UserIDEQ(userID),
			promotionactivityparticipation.StatusIn(PromotionParticipationStatusReserved, PromotionParticipationStatusGranted),
		).Count(ctx)
		if err != nil {
			return nil, fmt.Errorf("count subscription bonus participation: %w", err)
		}
		if used >= activity.MaxUsesPerUser || len(activity.Edges.PlanBonuses) == 0 {
			continue
		}
		bonus := activity.Edges.PlanBonuses[0]
		return &SubscriptionBonusBenefit{ActivityID: activity.ID, Days: bonus.BonusDays, EndsAt: activity.EndsAt}, nil
	}
	if expectedActivityID > 0 {
		return nil, infraerrors.Conflict("ACTIVITY_BENEFIT_CHANGED", "subscription bonus activity is no longer available; refresh and retry")
	}
	return nil, nil
}

func (s *PaymentService) reserveSubscriptionBonusForOrderTx(ctx context.Context, client *dbent.Client, orderID, userID, planID int64, benefit *SubscriptionBonusBenefit) error {
	if benefit == nil || benefit.ActivityID <= 0 || benefit.Days <= 0 {
		return nil
	}
	now := time.Now()
	if _, err := client.PromotionActivityParticipation.Create().
		SetActivityID(benefit.ActivityID).
		SetUserID(userID).
		SetOrderID(orderID).
		SetPlanID(planID).
		SetBonusDays(benefit.Days).
		SetStatus(PromotionParticipationStatusReserved).
		SetReservedAt(now).
		Save(ctx); err != nil {
		return fmt.Errorf("reserve subscription bonus: %w", err)
	}
	if err := s.writeAuditLogStrict(ctx, orderID, "SUBSCRIPTION_BONUS_RESERVED", fmt.Sprintf("user:%d", userID), map[string]any{
		"activityID": benefit.ActivityID,
		"planID":     planID,
		"bonusDays":  benefit.Days,
	}); err != nil {
		return fmt.Errorf("audit subscription bonus reservation: %w", err)
	}
	return nil
}

func releaseSubscriptionBonusForOrderTx(ctx context.Context, client *dbent.Client, orderID int64, reason string) (bool, error) {
	if client == nil || orderID <= 0 {
		return false, nil
	}
	now := time.Now()
	updated, err := client.PromotionActivityParticipation.Update().
		Where(
			promotionactivityparticipation.OrderIDEQ(orderID),
			promotionactivityparticipation.StatusEQ(PromotionParticipationStatusReserved),
		).
		SetStatus(PromotionParticipationStatusReleased).
		SetReleasedAt(now).
		SetReleaseReason(reason).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("release subscription bonus: %w", err)
	}
	return updated > 0, nil
}

// transitionPendingOrderWithBonusRelease keeps the terminal order transition
// and promotion reservation release in one transaction. The user lock matches
// order creation/reacquisition so a subsequent order cannot observe a stale
// reservation between the two writes.
func (s *PaymentService) transitionPendingOrderWithBonusRelease(ctx context.Context, order *dbent.PaymentOrder, targetStatus, reason string) (bool, error) {
	if s == nil || s.entClient == nil || order == nil || order.ID <= 0 {
		return false, infraerrors.BadRequest("INVALID_ORDER", "invalid payment order")
	}
	switch targetStatus {
	case OrderStatusCancelled, OrderStatusExpired, OrderStatusFailed:
	default:
		return false, infraerrors.BadRequest("INVALID_STATUS", "invalid terminal payment order status")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return false, fmt.Errorf("begin payment order terminal transition: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	// Always take the user lock. The order entity may be stale (for example, a
	// caller loaded it before the bonus snapshot was written), while the
	// participation row can still exist and must be released atomically with
	// the terminal transition.
	if err := lockPaymentUserForUpdate(txCtx, tx, order.UserID); err != nil {
		return false, err
	}
	updated, err := tx.PaymentOrder.Update().
		Where(paymentorder.IDEQ(order.ID), paymentorder.StatusEQ(OrderStatusPending)).
		SetStatus(targetStatus).
		Save(txCtx)
	if err != nil {
		return false, fmt.Errorf("update payment order terminal status: %w", err)
	}
	if updated == 0 {
		return false, nil
	}
	released, err := releaseSubscriptionBonusForOrderTx(txCtx, tx.Client(), order.ID, reason)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit payment order terminal transition: %w", err)
	}
	if released {
		s.writeAuditLog(ctx, order.ID, "SUBSCRIPTION_BONUS_RELEASED", "system", map[string]any{"reason": reason})
	}
	return true, nil
}

func (s *PaymentService) ensureSubscriptionBonusReservedForPaidOrder(ctx context.Context, order *dbent.PaymentOrder) error {
	if s == nil || s.entClient == nil || order == nil || order.SubscriptionBonusActivityID == nil || order.SubscriptionBonusDays <= 0 {
		return nil
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin subscription bonus reacquisition: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := lockPaymentUserForUpdate(txCtx, tx, order.UserID); err != nil {
		return err
	}
	participation, err := tx.PromotionActivityParticipation.Query().
		Where(promotionactivityparticipation.OrderIDEQ(order.ID)).
		Only(txCtx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return infraerrors.Conflict("SUBSCRIPTION_BONUS_RESERVATION_MISSING", "subscription bonus reservation is missing")
		}
		return fmt.Errorf("get subscription bonus reservation: %w", err)
	}
	if participation.Status == PromotionParticipationStatusReserved || participation.Status == PromotionParticipationStatusGranted {
		return nil
	}
	activityQuery := tx.PromotionActivity.Query().Where(promotionactivity.IDEQ(*order.SubscriptionBonusActivityID))
	if supportsForUpdate(tx.Client()) {
		// Serialize late-payment reacquisition with activity edits/deletion. A
		// shared lock still allows unrelated orders to reacquire concurrently.
		activityQuery = activityQuery.ForShare()
	}
	activity, err := activityQuery.Only(txCtx)
	if err != nil {
		return fmt.Errorf("get subscription bonus activity: %w", err)
	}
	used, err := tx.PromotionActivityParticipation.Query().Where(
		promotionactivityparticipation.ActivityIDEQ(activity.ID),
		promotionactivityparticipation.UserIDEQ(order.UserID),
		promotionactivityparticipation.StatusIn(PromotionParticipationStatusReserved, PromotionParticipationStatusGranted),
	).Count(txCtx)
	if err != nil {
		return fmt.Errorf("count subscription bonus participation: %w", err)
	}
	if used >= activity.MaxUsesPerUser {
		if err := s.writeAuditLogStrict(txCtx, order.ID, "SUBSCRIPTION_BONUS_REACQUIRE_FAILED", "system", map[string]any{"activityID": activity.ID, "reason": "user_limit_reached"}); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit subscription bonus reacquisition failure: %w", err)
		}
		return infraerrors.Conflict("SUBSCRIPTION_BONUS_REACQUIRE_FAILED", "subscription bonus reservation can no longer be reacquired")
	}
	updated, err := tx.PromotionActivityParticipation.Update().Where(
		promotionactivityparticipation.IDEQ(participation.ID),
		promotionactivityparticipation.StatusEQ(PromotionParticipationStatusReleased),
	).
		SetStatus(PromotionParticipationStatusReserved).
		SetReservedAt(time.Now()).
		ClearReleasedAt().
		ClearReleaseReason().
		Save(txCtx)
	if err != nil {
		return fmt.Errorf("reacquire subscription bonus reservation: %w", err)
	}
	if updated == 0 {
		return infraerrors.Conflict("SUBSCRIPTION_BONUS_REACQUIRE_FAILED", "subscription bonus reservation state changed")
	}
	if err := s.writeAuditLogStrict(txCtx, order.ID, "SUBSCRIPTION_BONUS_RESERVED", "system", map[string]any{"activityID": activity.ID, "reacquired": true}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit subscription bonus reacquisition: %w", err)
	}
	return nil
}

func (s *PaymentService) grantSubscriptionBonusForOrderTx(ctx context.Context, client *dbent.Client, order *dbent.PaymentOrder) error {
	if order == nil || order.SubscriptionBonusActivityID == nil || order.SubscriptionBonusDays <= 0 {
		return nil
	}
	participation, err := client.PromotionActivityParticipation.Query().
		Where(promotionactivityparticipation.OrderIDEQ(order.ID)).
		Only(ctx)
	if err != nil {
		return fmt.Errorf("get subscription bonus participation: %w", err)
	}
	if participation.Status == PromotionParticipationStatusGranted {
		return nil
	}
	if participation.Status != PromotionParticipationStatusReserved {
		return infraerrors.Conflict("SUBSCRIPTION_BONUS_NOT_RESERVED", "subscription bonus is not reserved")
	}
	updated, err := client.PromotionActivityParticipation.Update().Where(
		promotionactivityparticipation.IDEQ(participation.ID),
		promotionactivityparticipation.StatusEQ(PromotionParticipationStatusReserved),
	).
		SetStatus(PromotionParticipationStatusGranted).
		SetGrantedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("grant subscription bonus: %w", err)
	}
	if updated == 0 {
		return infraerrors.Conflict("SUBSCRIPTION_BONUS_STATE_CHANGED", "subscription bonus state changed")
	}
	if err := s.writeAuditLogStrict(ctx, order.ID, "SUBSCRIPTION_BONUS_GRANTED", "system", map[string]any{
		"activityID": *order.SubscriptionBonusActivityID,
		"bonusDays":  order.SubscriptionBonusDays,
	}); err != nil {
		return fmt.Errorf("audit subscription bonus grant: %w", err)
	}
	return nil
}

func subscriptionOrderTotalDays(order *dbent.PaymentOrder) (int, error) {
	if order == nil || order.SubscriptionDays == nil || *order.SubscriptionDays <= 0 {
		return 0, infraerrors.BadRequest("INVALID_STATUS", "missing subscription info")
	}
	baseDays := *order.SubscriptionDays
	bonusDays := order.SubscriptionBonusDays
	if bonusDays < 0 || baseDays > MaxValidityDays || bonusDays > MaxValidityDays-baseDays {
		return 0, infraerrors.BadRequest("SUBSCRIPTION_VALIDITY_TOO_LONG", "subscription validity exceeds the maximum")
	}
	return baseDays + bonusDays, nil
}
