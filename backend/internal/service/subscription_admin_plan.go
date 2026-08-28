package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionconcurrencyentitlement"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionearlyresetentitlement"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrSubscriptionPlanNotFound = infraerrors.NotFound(
		"SUBSCRIPTION_PLAN_NOT_FOUND",
		"subscription plan not found",
	)
	ErrSubscriptionPlanGroupMismatch = infraerrors.BadRequest(
		"SUBSCRIPTION_PLAN_GROUP_MISMATCH",
		"subscription plan does not belong to the subscription group",
	)
	ErrSubscriptionPlanUnavailable = infraerrors.BadRequest(
		"SUBSCRIPTION_PLAN_UNAVAILABLE",
		"subscription plan is not available for assignment",
	)
	ErrSubscriptionMultiplierRequired = infraerrors.BadRequest(
		"SUBSCRIPTION_MULTIPLIER_REQUIRED",
		"subscription multiplier is required",
	)
	ErrSubscriptionMultiplierDisabled = infraerrors.BadRequest(
		"SUBSCRIPTION_MULTIPLIER_DISABLED",
		"this subscription plan does not allow a custom multiplier",
	)
	ErrSubscriptionPlanSourceMismatch = infraerrors.Conflict(
		"SUBSCRIPTION_PLAN_SOURCE_MISMATCH",
		"subscription is already linked to a different plan",
	)
)

// AssignPlanSubscriptionInput describes an admin assignment backed by a
// subscription plan. ValidityDays may override the plan's normal duration.
type AssignPlanSubscriptionInput struct {
	UserID       int64
	GroupID      int64
	PlanID       int64
	Multiplier   *int
	ValidityDays int
	AssignedBy   int64
	Notes        string
}

// UpdateSubscriptionMultiplierInput updates the custom multiplier without
// changing the subscription's validity or recorded usage.
type UpdateSubscriptionMultiplierInput struct {
	SubscriptionID int64
	PlanID         int64
	Multiplier     int
}

// AssignPlanSubscription assigns the full plan semantics, including custom
// multiplier metadata, plan concurrency, and the early-reset policy.
func (s *SubscriptionService) AssignPlanSubscription(ctx context.Context, input *AssignPlanSubscriptionInput) (*UserSubscription, error) {
	if input == nil {
		return nil, ErrSubscriptionNilInput
	}
	assignment, err := s.buildPlanAssignmentInput(
		ctx,
		input.GroupID,
		input.PlanID,
		input.Multiplier,
		input.ValidityDays,
		input.AssignedBy,
		input.Notes,
	)
	if err != nil {
		return nil, err
	}
	assignment.UserID = input.UserID
	var subscription *UserSubscription
	var reused bool
	err = s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		var assignErr error
		subscription, reused, assignErr = s.assignSubscriptionWithReuseMode(txCtx, assignment, true)
		return assignErr
	})
	if err != nil {
		return nil, err
	}
	if !reused {
		s.maybeInvalidateAssignmentCaches(assignment.UserID, assignment.GroupID, false)
	}
	return subscription, nil
}

func (s *SubscriptionService) buildPlanAssignmentInput(
	ctx context.Context,
	groupID int64,
	planID int64,
	multiplier *int,
	validityDays int,
	assignedBy int64,
	notes string,
) (*AssignSubscriptionInput, error) {
	plan, group, resolvedMultiplier, custom, err := s.resolveAdminPlanSelection(ctx, groupID, planID, multiplier)
	if err != nil {
		return nil, err
	}
	if validityDays <= 0 {
		validityDays, err = validateSubscriptionPlanValidity(plan.ValidityDays, plan.ValidityUnit)
		if err != nil {
			return nil, err
		}
	}
	concurrency, err := subscriptionOrderConcurrency(plan)
	if err != nil {
		return nil, err
	}
	earlyResetEnabled := plan.EarlyResetEnabled
	earlyResetDurationDays := 0
	if earlyResetEnabled {
		earlyResetDurationDays = plan.EarlyResetDurationDays
	}
	assignment := &AssignSubscriptionInput{
		GroupID:                plan.GroupID,
		ValidityDays:           validityDays,
		AssignedBy:             assignedBy,
		Notes:                  notes,
		PlanConcurrency:        &concurrency,
		EarlyResetEnabled:      &earlyResetEnabled,
		EarlyResetDurationDays: &earlyResetDurationDays,
	}
	if custom {
		assignment.CustomMultiplier = &resolvedMultiplier
		assignment.CustomSourcePlanID = &plan.ID
		assignment.CustomSourceGroupID = &plan.GroupID
		assignment.CustomDisplayName = customSubscriptionGroupName(group.Name, resolvedMultiplier)
	}
	return assignment, nil
}

func (s *SubscriptionService) resolveAdminPlanSelection(
	ctx context.Context,
	groupID int64,
	planID int64,
	requestedMultiplier *int,
) (*dbent.SubscriptionPlan, *Group, int, bool, error) {
	if s == nil || s.entClient == nil || s.groupRepo == nil {
		return nil, nil, 0, false, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "subscription service unavailable")
	}
	if planID <= 0 {
		return nil, nil, 0, false, ErrSubscriptionPlanNotFound
	}
	plan, err := s.entClient.SubscriptionPlan.Get(ctx, planID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil, 0, false, ErrSubscriptionPlanNotFound
		}
		return nil, nil, 0, false, fmt.Errorf("get subscription plan: %w", err)
	}
	if groupID <= 0 {
		groupID = plan.GroupID
	}
	if plan.GroupID != groupID {
		return nil, nil, 0, false, ErrSubscriptionPlanGroupMismatch
	}
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, nil, 0, false, fmt.Errorf("get subscription plan group: %w", err)
	}
	if !group.IsActive() || !group.IsSubscriptionType() || group.IsCustomSubscriptionGroup {
		return nil, nil, 0, false, ErrSubscriptionPlanUnavailable
	}
	resolvedMultiplier, custom, err := resolveAdminPlanMultiplier(plan, requestedMultiplier)
	if err != nil {
		return nil, nil, 0, false, err
	}
	return plan, group, resolvedMultiplier, custom, nil
}

func resolveAdminPlanMultiplier(plan *dbent.SubscriptionPlan, requested *int) (int, bool, error) {
	if plan == nil {
		return 0, false, ErrSubscriptionPlanNotFound
	}
	if !plan.CustomMultiplierEnabled {
		if requested != nil && *requested != 1 {
			return 0, false, ErrSubscriptionMultiplierDisabled
		}
		return 1, false, nil
	}
	if requested == nil {
		return 0, false, ErrSubscriptionMultiplierRequired
	}
	minMultiplier, maxMultiplier, err := normalizePlanCustomMultiplierConfig(
		true,
		plan.CustomMultiplierMin,
		plan.CustomMultiplierMax,
	)
	if err != nil {
		return 0, false, err
	}
	if *requested < minMultiplier || *requested > maxMultiplier {
		return 0, false, infraerrors.BadRequest(
			"INVALID_SUBSCRIPTION_MULTIPLIER",
			"subscription multiplier is out of range",
		).WithMetadata(map[string]string{
			"min": strconv.Itoa(minMultiplier),
			"max": strconv.Itoa(maxMultiplier),
		})
	}
	return *requested, true, nil
}

// UpdateSubscriptionMultiplier links a plain group assignment to its plan, or
// changes the multiplier of an existing plan-backed subscription. Existing
// plan-backed subscriptions cannot be switched to a different source plan.
func (s *SubscriptionService) UpdateSubscriptionMultiplier(ctx context.Context, input *UpdateSubscriptionMultiplierInput) (*UserSubscription, error) {
	if input == nil || input.SubscriptionID <= 0 {
		return nil, ErrSubscriptionNilInput
	}
	sub, err := s.userSubRepo.GetByID(ctx, input.SubscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}
	now := time.Now()
	if sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(now) {
		return nil, ErrSubscriptionExpired
	}
	if sub.Group != nil && sub.Group.IsCustomSubscriptionGroup {
		return nil, ErrSubscriptionPlanUnavailable
	}
	sourceGroupID := sub.GroupID
	if sub.CustomSourceGroupID != nil {
		sourceGroupID = *sub.CustomSourceGroupID
	}
	plan, group, multiplier, custom, err := s.resolveAdminPlanSelection(
		ctx,
		sourceGroupID,
		input.PlanID,
		&input.Multiplier,
	)
	if err != nil {
		return nil, err
	}
	if !custom {
		return nil, ErrSubscriptionMultiplierDisabled
	}
	if sub.HasActiveVirtualCustomEntitlementAt(now) && sub.CustomSourcePlanID != nil && *sub.CustomSourcePlanID != plan.ID {
		return nil, ErrSubscriptionPlanSourceMismatch
	}

	customExpiresAt := sub.ExpiresAt
	if sub.CustomExpiresAt != nil && sub.CustomExpiresAt.After(now) && sub.CustomExpiresAt.Before(customExpiresAt) {
		customExpiresAt = *sub.CustomExpiresAt
	}
	updated := *sub
	updated.CustomMultiplier = &multiplier
	updated.CustomSourcePlanID = &plan.ID
	updated.CustomSourceGroupID = &plan.GroupID
	updated.CustomExpiresAt = &customExpiresAt
	updated.CustomDisplayName = customSubscriptionGroupName(group.Name, multiplier)
	updated.EarlyResetEnabled = plan.EarlyResetEnabled
	updated.EarlyResetDurationDays = 0
	if plan.EarlyResetEnabled {
		updated.EarlyResetDurationDays = plan.EarlyResetDurationDays
	}

	err = s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		if err := s.userSubRepo.Update(txCtx, &updated); err != nil {
			return fmt.Errorf("update subscription multiplier: %w", err)
		}
		if err := s.upsertAdminPlanConcurrencyEntitlement(txCtx, &updated, plan.Concurrency, now, customExpiresAt); err != nil {
			return fmt.Errorf("update subscription concurrency entitlement: %w", err)
		}
		if err := s.upsertAdminEarlyResetEntitlement(txCtx, &updated, plan, now, customExpiresAt); err != nil {
			return fmt.Errorf("update subscription early reset entitlement: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.invalidateSubscriptionCaches(ctx, updated.UserID, updated.GroupID)
	return s.GetByID(ctx, updated.ID)
}

func (s *SubscriptionService) upsertAdminPlanConcurrencyEntitlement(
	ctx context.Context,
	sub *UserSubscription,
	concurrency int,
	startsAt time.Time,
	expiresAt time.Time,
) error {
	if s == nil || s.entClient == nil || sub == nil || concurrency <= 0 || !startsAt.Before(expiresAt) {
		return nil
	}
	client := s.entClient
	if tx := dbent.TxFromContext(ctx); tx != nil {
		client = tx.Client()
	}
	entitlement, err := client.SubscriptionConcurrencyEntitlement.Query().
		Where(
			subscriptionconcurrencyentitlement.SubscriptionIDEQ(sub.ID),
			subscriptionconcurrencyentitlement.SourceOrderIDIsNil(),
			subscriptionconcurrencyentitlement.StartsAtLTE(startsAt),
			subscriptionconcurrencyentitlement.ExpiresAtGT(startsAt),
		).
		Order(dbent.Desc(subscriptionconcurrencyentitlement.FieldStartsAt), dbent.Desc(subscriptionconcurrencyentitlement.FieldID)).
		First(ctx)
	if err == nil {
		return client.SubscriptionConcurrencyEntitlement.UpdateOneID(entitlement.ID).
			SetConcurrency(concurrency).
			SetExpiresAt(expiresAt).
			Exec(ctx)
	}
	if !dbent.IsNotFound(err) {
		return err
	}
	_, err = client.SubscriptionConcurrencyEntitlement.Create().
		SetUserID(sub.UserID).
		SetSubscriptionID(sub.ID).
		SetConcurrency(concurrency).
		SetStartsAt(startsAt).
		SetExpiresAt(expiresAt).
		Save(ctx)
	return err
}

func (s *SubscriptionService) upsertAdminEarlyResetEntitlement(
	ctx context.Context,
	sub *UserSubscription,
	plan *dbent.SubscriptionPlan,
	startsAt time.Time,
	expiresAt time.Time,
) error {
	if s == nil || s.entClient == nil || sub == nil || plan == nil || !startsAt.Before(expiresAt) {
		return nil
	}
	client := s.entClient
	if tx := dbent.TxFromContext(ctx); tx != nil {
		client = tx.Client()
	}
	durationDays := 0
	if plan.EarlyResetEnabled {
		durationDays = plan.EarlyResetDurationDays
	}
	entitlement, err := client.SubscriptionEarlyResetEntitlement.Query().
		Where(
			subscriptionearlyresetentitlement.SubscriptionIDEQ(sub.ID),
			subscriptionearlyresetentitlement.SourceOrderIDIsNil(),
			subscriptionearlyresetentitlement.CustomTermEQ(true),
			subscriptionearlyresetentitlement.StartsAtLTE(startsAt),
			subscriptionearlyresetentitlement.ExpiresAtGT(startsAt),
		).
		Order(dbent.Desc(subscriptionearlyresetentitlement.FieldStartsAt), dbent.Desc(subscriptionearlyresetentitlement.FieldID)).
		First(ctx)
	if err == nil {
		return client.SubscriptionEarlyResetEntitlement.UpdateOneID(entitlement.ID).
			SetEnabled(plan.EarlyResetEnabled).
			SetDurationDays(durationDays).
			SetExpiresAt(expiresAt).
			Exec(ctx)
	}
	if !dbent.IsNotFound(err) {
		return err
	}
	_, err = client.SubscriptionEarlyResetEntitlement.Create().
		SetUserID(sub.UserID).
		SetSubscriptionID(sub.ID).
		SetEnabled(plan.EarlyResetEnabled).
		SetDurationDays(durationDays).
		SetCustomTerm(true).
		SetStartsAt(startsAt).
		SetExpiresAt(expiresAt).
		Save(ctx)
	return err
}
