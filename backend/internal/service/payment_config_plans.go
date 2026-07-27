package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionconcurrencyentitlement"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const maxPlanConcurrency = int(^uint32(0) >> 1)

// validatePlanRequired checks that all required fields for a plan are provided.
func validatePlanRequired(name string, groupID int64, price float64, validityDays int, concurrency int, validityUnit string, originalPrice *float64) error {
	if strings.TrimSpace(name) == "" {
		return infraerrors.BadRequest("PLAN_NAME_REQUIRED", "plan name is required")
	}
	if groupID <= 0 {
		return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "group is required")
	}
	if price <= 0 {
		return infraerrors.BadRequest("PLAN_PRICE_INVALID", "price must be > 0")
	}
	if validityDays <= 0 {
		return infraerrors.BadRequest("PLAN_VALIDITY_REQUIRED", "validity days must be > 0")
	}
	if concurrency <= 0 || concurrency > maxPlanConcurrency {
		return infraerrors.BadRequest("PLAN_CONCURRENCY_INVALID", "concurrency must be between 1 and 2147483647")
	}
	if strings.TrimSpace(validityUnit) == "" {
		return infraerrors.BadRequest("PLAN_VALIDITY_UNIT_REQUIRED", "validity unit is required")
	}
	if originalPrice != nil && *originalPrice < 0 {
		return infraerrors.BadRequest("PLAN_ORIGINAL_PRICE_INVALID", "original price must be >= 0")
	}
	return nil
}

func (s *PaymentConfigService) validateSubscriptionPlanGroup(ctx context.Context, groupID int64) error {
	if groupID <= 0 {
		return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "group is required")
	}
	if s == nil || s.entClient == nil {
		return infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "payment config service unavailable")
	}
	g, err := s.entClient.Group.Get(ctx, groupID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return infraerrors.NotFound("PLAN_GROUP_NOT_FOUND", "subscription plan group not found")
		}
		return fmt.Errorf("get subscription plan group: %w", err)
	}
	if g.Status != payment.EntityStatusActive {
		return infraerrors.BadRequest("PLAN_GROUP_INACTIVE", "subscription plan group must be active")
	}
	if g.SubscriptionType != SubscriptionTypeSubscription {
		return infraerrors.BadRequest("PLAN_GROUP_TYPE_MISMATCH", "subscription plan group must be subscription type")
	}
	if g.IsCustomSubscriptionGroup {
		return infraerrors.BadRequest("PLAN_GROUP_CUSTOM_NOT_ALLOWED", "subscription plan group must be a source group, not a custom subscription group")
	}
	return nil
}

func normalizePlanCustomMultiplierConfig(enabled bool, minValue int, maxValue int) (int, int, error) {
	if !enabled {
		if minValue < minCustomSubscriptionMultiplier {
			minValue = minCustomSubscriptionMultiplier
		}
		if minValue > maxCustomSubscriptionMultiplier {
			minValue = maxCustomSubscriptionMultiplier
		}
		if maxValue < minValue {
			maxValue = minValue
		}
		if maxValue > maxCustomSubscriptionMultiplier {
			maxValue = maxCustomSubscriptionMultiplier
		}
		return minValue, maxValue, nil
	}
	if minValue < minCustomSubscriptionMultiplier {
		return 0, 0, infraerrors.BadRequest("PLAN_CUSTOM_MULTIPLIER_MIN_INVALID", fmt.Sprintf("custom multiplier min must be >= %d", minCustomSubscriptionMultiplier))
	}
	if minValue > maxCustomSubscriptionMultiplier {
		return 0, 0, infraerrors.BadRequest("PLAN_CUSTOM_MULTIPLIER_MIN_INVALID", fmt.Sprintf("custom multiplier min must be <= %d", maxCustomSubscriptionMultiplier))
	}
	if maxValue < minValue {
		return 0, 0, infraerrors.BadRequest("PLAN_CUSTOM_MULTIPLIER_MAX_INVALID", "custom multiplier max must be >= min")
	}
	if maxValue > maxCustomSubscriptionMultiplier {
		return 0, 0, infraerrors.BadRequest("PLAN_CUSTOM_MULTIPLIER_MAX_INVALID", fmt.Sprintf("custom multiplier max must be <= %d", maxCustomSubscriptionMultiplier))
	}
	return minValue, maxValue, nil
}

func normalizePlanEarlyResetConfig(enabled bool, durationDays int) (int, error) {
	if durationDays == 0 && !enabled {
		return 1, nil
	}
	if durationDays <= 0 || durationDays > MaxValidityDays {
		return 0, infraerrors.BadRequest("PLAN_EARLY_RESET_DURATION_INVALID", "early reset duration days must be between 1 and 36500")
	}
	return durationDays, nil
}

// validatePlanPatch validates only the non-nil fields in a patch update.
func validatePlanPatch(req UpdatePlanRequest) error {
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return infraerrors.BadRequest("PLAN_NAME_REQUIRED", "plan name is required")
	}
	if req.GroupID != nil && *req.GroupID <= 0 {
		return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "group is required")
	}
	if req.Price != nil && *req.Price <= 0 {
		return infraerrors.BadRequest("PLAN_PRICE_INVALID", "price must be > 0")
	}
	if req.ValidityDays != nil && *req.ValidityDays <= 0 {
		return infraerrors.BadRequest("PLAN_VALIDITY_REQUIRED", "validity days must be > 0")
	}
	if req.Concurrency != nil && (*req.Concurrency <= 0 || *req.Concurrency > maxPlanConcurrency) {
		return infraerrors.BadRequest("PLAN_CONCURRENCY_INVALID", "concurrency must be between 1 and 2147483647")
	}
	if req.ValidityUnit != nil && strings.TrimSpace(*req.ValidityUnit) == "" {
		return infraerrors.BadRequest("PLAN_VALIDITY_UNIT_REQUIRED", "validity unit is required")
	}
	if req.OriginalPrice != nil && *req.OriginalPrice < 0 {
		return infraerrors.BadRequest("PLAN_ORIGINAL_PRICE_INVALID", "original price must be >= 0")
	}
	if req.EarlyResetDurationDays != nil && (*req.EarlyResetDurationDays <= 0 || *req.EarlyResetDurationDays > MaxValidityDays) {
		return infraerrors.BadRequest("PLAN_EARLY_RESET_DURATION_INVALID", "early reset duration days must be between 1 and 36500")
	}
	return nil
}

// --- Plan CRUD ---

// PlanGroupInfo holds the group details needed for subscription plan display.
type PlanGroupInfo struct {
	Platform        string   `json:"platform"`
	Name            string   `json:"name"`
	RateMultiplier  float64  `json:"rate_multiplier"`
	DailyLimitUSD   *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD  *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD *float64 `json:"monthly_limit_usd"`
	ModelScopes     []string `json:"supported_model_scopes"`
}

// GetGroupPlatformMap returns a map of group_id → platform for the given plans.
func (s *PaymentConfigService) GetGroupPlatformMap(ctx context.Context, plans []*dbent.SubscriptionPlan) map[int64]string {
	info := s.GetGroupInfoMap(ctx, plans)
	m := make(map[int64]string, len(info))
	for id, gi := range info {
		m[id] = gi.Platform
	}
	return m
}

// GetGroupInfoMap returns a map of group_id → PlanGroupInfo for the given plans.
func (s *PaymentConfigService) GetGroupInfoMap(ctx context.Context, plans []*dbent.SubscriptionPlan) map[int64]PlanGroupInfo {
	ids := make([]int64, 0, len(plans))
	seen := make(map[int64]bool)
	for _, p := range plans {
		if !seen[p.GroupID] {
			seen[p.GroupID] = true
			ids = append(ids, p.GroupID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	groups, err := s.entClient.Group.Query().Where(group.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil
	}
	m := make(map[int64]PlanGroupInfo, len(groups))
	for _, g := range groups {
		m[int64(g.ID)] = PlanGroupInfo{
			Platform:        g.Platform,
			Name:            g.Name,
			RateMultiplier:  g.RateMultiplier,
			DailyLimitUSD:   g.DailyLimitUsd,
			WeeklyLimitUSD:  g.WeeklyLimitUsd,
			MonthlyLimitUSD: g.MonthlyLimitUsd,
			ModelScopes:     g.SupportedModelScopes,
		}
	}
	return m
}

func (s *PaymentConfigService) ListPlans(ctx context.Context) ([]*dbent.SubscriptionPlan, error) {
	return s.entClient.SubscriptionPlan.Query().Order(subscriptionplan.BySortOrder()).All(ctx)
}

func (s *PaymentConfigService) ListPlansForSale(ctx context.Context) ([]*dbent.SubscriptionPlan, error) {
	return s.entClient.SubscriptionPlan.Query().Where(subscriptionplan.ForSaleEQ(true)).Order(subscriptionplan.BySortOrder()).All(ctx)
}

func (s *PaymentConfigService) CreatePlan(ctx context.Context, req CreatePlanRequest) (*dbent.SubscriptionPlan, error) {
	if req.Concurrency == 0 {
		req.Concurrency = 1
	}
	if err := validatePlanRequired(req.Name, req.GroupID, req.Price, req.ValidityDays, req.Concurrency, req.ValidityUnit, req.OriginalPrice); err != nil {
		return nil, err
	}
	if err := s.validateSubscriptionPlanGroup(ctx, req.GroupID); err != nil {
		return nil, err
	}
	minValue, maxValue, err := normalizePlanCustomMultiplierConfig(req.CustomMultiplierEnabled, req.CustomMultiplierMin, req.CustomMultiplierMax)
	if err != nil {
		return nil, err
	}
	earlyResetDurationDays, err := normalizePlanEarlyResetConfig(req.EarlyResetEnabled, req.EarlyResetDurationDays)
	if err != nil {
		return nil, err
	}
	b := s.entClient.SubscriptionPlan.Create().
		SetGroupID(req.GroupID).SetName(req.Name).SetDescription(req.Description).
		SetPrice(req.Price).SetValidityDays(req.ValidityDays).SetConcurrency(req.Concurrency).SetValidityUnit(req.ValidityUnit).
		SetEarlyResetEnabled(req.EarlyResetEnabled).
		SetEarlyResetDurationDays(earlyResetDurationDays).
		SetFeatures(req.Features).SetProductName(req.ProductName).
		SetForSale(req.ForSale).SetSortOrder(req.SortOrder).
		SetCustomMultiplierEnabled(req.CustomMultiplierEnabled).
		SetCustomMultiplierMin(minValue).
		SetCustomMultiplierMax(maxValue)
	if req.OriginalPrice != nil {
		b.SetOriginalPrice(*req.OriginalPrice)
	}
	return b.Save(ctx)
}

// UpdatePlan updates a subscription plan by ID (patch semantics).
// NOTE: This function exceeds 30 lines due to per-field nil-check patch update boilerplate
// plus a validation guard for non-nil fields.
func (s *PaymentConfigService) UpdatePlan(ctx context.Context, id int64, req UpdatePlanRequest) (*dbent.SubscriptionPlan, error) {
	if err := validatePlanPatch(req); err != nil {
		return nil, err
	}
	if req.GroupID != nil {
		if err := s.validateSubscriptionPlanGroup(ctx, *req.GroupID); err != nil {
			return nil, err
		}
	}
	current, err := s.entClient.SubscriptionPlan.Get(ctx, id)
	if err != nil {
		return nil, infraerrors.NotFound("PLAN_NOT_FOUND", "subscription plan not found")
	}
	enabled := current.CustomMultiplierEnabled
	minValue := current.CustomMultiplierMin
	maxValue := current.CustomMultiplierMax
	if req.CustomMultiplierEnabled != nil {
		enabled = *req.CustomMultiplierEnabled
	}
	if req.CustomMultiplierMin != nil {
		minValue = *req.CustomMultiplierMin
	}
	if req.CustomMultiplierMax != nil {
		maxValue = *req.CustomMultiplierMax
	}
	minValue, maxValue, err = normalizePlanCustomMultiplierConfig(enabled, minValue, maxValue)
	if err != nil {
		return nil, err
	}
	earlyResetEnabled := current.EarlyResetEnabled
	earlyResetDurationDays := current.EarlyResetDurationDays
	if req.EarlyResetEnabled != nil {
		earlyResetEnabled = *req.EarlyResetEnabled
	}
	if req.EarlyResetDurationDays != nil {
		earlyResetDurationDays = *req.EarlyResetDurationDays
	}
	earlyResetDurationDays, err = normalizePlanEarlyResetConfig(earlyResetEnabled, earlyResetDurationDays)
	if err != nil {
		return nil, err
	}
	u := s.entClient.SubscriptionPlan.UpdateOneID(id)
	if req.GroupID != nil {
		u.SetGroupID(*req.GroupID)
	}
	if req.Name != nil {
		u.SetName(*req.Name)
	}
	if req.Description != nil {
		u.SetDescription(*req.Description)
	}
	if req.Price != nil {
		u.SetPrice(*req.Price)
	}
	if req.OriginalPrice != nil {
		u.SetOriginalPrice(*req.OriginalPrice)
	}
	if req.ValidityDays != nil {
		u.SetValidityDays(*req.ValidityDays)
	}
	if req.Concurrency != nil {
		u.SetConcurrency(*req.Concurrency)
	}
	if req.ValidityUnit != nil {
		u.SetValidityUnit(*req.ValidityUnit)
	}
	if req.EarlyResetEnabled != nil || req.EarlyResetDurationDays != nil {
		u.SetEarlyResetEnabled(earlyResetEnabled)
		u.SetEarlyResetDurationDays(earlyResetDurationDays)
	}
	if req.Features != nil {
		u.SetFeatures(*req.Features)
	}
	if req.ProductName != nil {
		u.SetProductName(*req.ProductName)
	}
	if req.ForSale != nil {
		u.SetForSale(*req.ForSale)
	}
	if req.SortOrder != nil {
		u.SetSortOrder(*req.SortOrder)
	}
	if req.CustomMultiplierEnabled != nil {
		u.SetCustomMultiplierEnabled(enabled)
	}
	if req.CustomMultiplierMin != nil || req.CustomMultiplierMax != nil || req.CustomMultiplierEnabled != nil {
		u.SetCustomMultiplierMin(minValue)
		u.SetCustomMultiplierMax(maxValue)
	}
	plan, err := u.Save(ctx)
	if err != nil {
		return nil, err
	}
	if req.Concurrency != nil && *req.Concurrency != current.Concurrency {
		s.raiseActiveConcurrencyEntitlements(ctx, plan)
	}
	return plan, nil
}

// raiseActiveConcurrencyEntitlements lifts unexpired concurrency entitlements of
// the plan's group up to the plan's new value.
//
// Entitlements are snapshots taken when a subscription is created or extended, so
// editing a plan alone would leave existing subscribers on the value they bought
// until their next renewal. Terms that have not started yet are included as well:
// an early renewal queues its term behind the current one, and that term would
// otherwise drop the user back to the old value once it takes over.
//
// Snapshots already at or above the new value are left alone: a higher limit is
// either a deliberate grant or a richer purchase, and a plan edit must not claw it
// back. Failures are logged rather than returned — the plan itself is already
// persisted, so the edit must not appear to have failed.
func (s *PaymentConfigService) raiseActiveConcurrencyEntitlements(ctx context.Context, plan *dbent.SubscriptionPlan) {
	if s == nil || s.entClient == nil || plan == nil || plan.Concurrency <= 0 {
		return
	}
	now := time.Now()
	pending, err := s.entClient.SubscriptionConcurrencyEntitlement.Query().
		Where(
			subscriptionconcurrencyentitlement.ConcurrencyLT(plan.Concurrency),
			subscriptionconcurrencyentitlement.ExpiresAtGT(now),
			subscriptionconcurrencyentitlement.HasSubscriptionWith(
				usersubscription.GroupIDEQ(plan.GroupID),
				usersubscription.StatusEQ(SubscriptionStatusActive),
				usersubscription.ExpiresAtGT(now),
				usersubscription.DeletedAtIsNil(),
			),
		).
		Select(
			subscriptionconcurrencyentitlement.FieldID,
			subscriptionconcurrencyentitlement.FieldUserID,
		).
		All(ctx)
	if err != nil {
		slog.Error("failed to load concurrency entitlements for plan update",
			"plan_id", plan.ID, "group_id", plan.GroupID, "error", err)
		return
	}
	if len(pending) == 0 {
		return
	}

	ids := make([]int64, 0, len(pending))
	userIDs := make([]int64, 0, len(pending))
	seen := make(map[int64]struct{}, len(pending))
	for _, entitlement := range pending {
		ids = append(ids, entitlement.ID)
		if _, ok := seen[entitlement.UserID]; ok {
			continue
		}
		seen[entitlement.UserID] = struct{}{}
		userIDs = append(userIDs, entitlement.UserID)
	}

	affected, err := s.entClient.SubscriptionConcurrencyEntitlement.Update().
		Where(subscriptionconcurrencyentitlement.IDIn(ids...)).
		SetConcurrency(plan.Concurrency).
		Save(ctx)
	if err != nil {
		slog.Error("failed to raise concurrency entitlements for plan update",
			"plan_id", plan.ID, "group_id", plan.GroupID, "concurrency", plan.Concurrency, "error", err)
		return
	}
	slog.Info("raised concurrency entitlements after plan update",
		"plan_id", plan.ID, "group_id", plan.GroupID, "concurrency", plan.Concurrency,
		"entitlements", affected, "users", len(userIDs))

	// Auth snapshots cache the entitlement list, so without this the new limit
	// only applies once the cached snapshot expires.
	if s.authCacheInvalidator != nil {
		for _, userID := range userIDs {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
		}
	}
}

func (s *PaymentConfigService) DeletePlan(ctx context.Context, id int64) error {
	count, err := s.countPendingOrdersByPlan(ctx, id)
	if err != nil {
		return fmt.Errorf("check pending orders: %w", err)
	}
	if count > 0 {
		return infraerrors.Conflict("PENDING_ORDERS",
			fmt.Sprintf("this plan has %d in-progress orders and cannot be deleted — wait for orders to complete first", count))
	}
	return s.entClient.SubscriptionPlan.DeleteOneID(id).Exec(ctx)
}

// GetPlan returns a subscription plan by ID.
func (s *PaymentConfigService) GetPlan(ctx context.Context, id int64) (*dbent.SubscriptionPlan, error) {
	plan, err := s.entClient.SubscriptionPlan.Get(ctx, id)
	if err != nil {
		return nil, infraerrors.NotFound("PLAN_NOT_FOUND", "subscription plan not found")
	}
	return plan, nil
}
