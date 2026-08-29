package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionconcurrencyentitlement"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionearlyresetentitlement"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
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
		if assignErr != nil || !reused {
			return assignErr
		}
		return s.validateReusedPlanConcurrency(txCtx, subscription, assignment)
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
	client := s.entClient
	if tx := dbent.TxFromContext(ctx); tx != nil {
		client = tx.Client()
	}
	plan, err := client.SubscriptionPlan.Get(ctx, planID)
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

func (s *SubscriptionService) validateReusedPlanConcurrency(ctx context.Context, sub *UserSubscription, input *AssignSubscriptionInput) error {
	if sub == nil || input == nil || input.PlanConcurrency == nil {
		return nil
	}
	now := time.Now()
	if legacy, ok := sub.ActivePlanConcurrencyEntitlementAt(now); ok && legacy.Concurrency == *input.PlanConcurrency {
		return nil
	}
	client := s.entClient
	if tx := dbent.TxFromContext(ctx); tx != nil {
		client = tx.Client()
	}
	matched, err := client.SubscriptionConcurrencyEntitlement.Query().
		Where(
			subscriptionconcurrencyentitlement.SubscriptionIDEQ(sub.ID),
			subscriptionconcurrencyentitlement.ConcurrencyEQ(*input.PlanConcurrency),
			subscriptionconcurrencyentitlement.StartsAtLTE(now),
			subscriptionconcurrencyentitlement.ExpiresAtGT(now),
		).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("check existing subscription plan concurrency: %w", err)
	}
	if matched {
		return nil
	}
	return ErrSubscriptionAssignConflict.WithMetadata(map[string]string{
		"conflict_reason": "plan_concurrency_mismatch",
	})
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
	if s == nil || s.entClient == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "subscription service unavailable")
	}
	var subscriptionID, userID, groupID int64
	err := s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		client := s.entClient
		if tx := dbent.TxFromContext(txCtx); tx != nil {
			client = tx.Client()
		}
		query := client.UserSubscription.Query().
			Where(usersubscription.IDEQ(input.SubscriptionID)).
			WithGroup()
		if supportsForUpdate(client) {
			query = query.ForUpdate()
		}
		current, lockErr := query.Only(txCtx)
		if lockErr != nil {
			if dbent.IsNotFound(lockErr) {
				return ErrSubscriptionNotFound
			}
			return fmt.Errorf("lock subscription for multiplier update: %w", lockErr)
		}

		now := time.Now()
		if current.Status != SubscriptionStatusActive || !current.ExpiresAt.After(now) {
			return ErrSubscriptionExpired
		}
		if current.Edges.Group != nil && current.Edges.Group.IsCustomSubscriptionGroup {
			return ErrSubscriptionPlanUnavailable
		}
		sourceGroupID := current.GroupID
		if current.CustomSourceGroupID != nil {
			sourceGroupID = *current.CustomSourceGroupID
		}
		plan, group, multiplier, custom, resolveErr := s.resolveAdminPlanSelection(
			txCtx,
			sourceGroupID,
			input.PlanID,
			&input.Multiplier,
		)
		if resolveErr != nil {
			return resolveErr
		}
		if !custom {
			return ErrSubscriptionMultiplierDisabled
		}
		if current.CustomExpiresAt != nil && current.CustomExpiresAt.After(now) && current.CustomSourcePlanID != nil && *current.CustomSourcePlanID != plan.ID {
			return ErrSubscriptionPlanSourceMismatch
		}

		customExpiresAt := current.ExpiresAt
		if current.CustomExpiresAt != nil && current.CustomExpiresAt.After(now) && current.CustomExpiresAt.Before(customExpiresAt) {
			customExpiresAt = *current.CustomExpiresAt
		}
		earlyResetDurationDays := 0
		if plan.EarlyResetEnabled {
			earlyResetDurationDays = plan.EarlyResetDurationDays
		}
		// Only update fields owned by the selected plan. Usage counters, status,
		// validity, and reset windows may be changing concurrently on hot paths.
		if _, updateErr := client.UserSubscription.UpdateOneID(current.ID).
			SetCustomMultiplier(multiplier).
			SetCustomSourcePlanID(plan.ID).
			SetCustomSourceGroupID(plan.GroupID).
			SetCustomExpiresAt(customExpiresAt).
			SetCustomDisplayName(customSubscriptionGroupName(group.Name, multiplier)).
			SetEarlyResetEnabled(plan.EarlyResetEnabled).
			SetEarlyResetDurationDays(earlyResetDurationDays).
			Save(txCtx); updateErr != nil {
			return fmt.Errorf("update subscription multiplier: %w", updateErr)
		}
		updated := &UserSubscription{ID: current.ID, UserID: current.UserID}
		if err := s.upsertAdminPlanConcurrencyEntitlement(txCtx, updated, plan.Concurrency, now, customExpiresAt); err != nil {
			return fmt.Errorf("update subscription concurrency entitlement: %w", err)
		}
		if err := s.upsertAdminEarlyResetEntitlement(txCtx, updated, plan, now, customExpiresAt); err != nil {
			return fmt.Errorf("update subscription early reset entitlement: %w", err)
		}
		subscriptionID = current.ID
		userID = current.UserID
		groupID = current.GroupID
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.invalidateSubscriptionCaches(ctx, userID, groupID)
	return s.GetByID(ctx, subscriptionID)
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
