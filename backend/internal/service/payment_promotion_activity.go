package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivity"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivityparticipation"
	"github.com/Wei-Shaw/sub2api/ent/promotionactivityplan"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	PromotionActivityTypeSubscriptionBonusDays = "subscription_bonus_days"

	PromotionActivityStatusDisabled  = "disabled"
	PromotionActivityStatusScheduled = "scheduled"
	PromotionActivityStatusActive    = "active"
	PromotionActivityStatusEnded     = "ended"

	PromotionParticipationStatusReserved = "reserved"
	PromotionParticipationStatusGranted  = "granted"
	PromotionParticipationStatusReleased = "released"
)

type PromotionActivityPlanInput struct {
	PlanID    int64 `json:"plan_id"`
	BonusDays int   `json:"bonus_days"`
}

type UpsertPromotionActivityRequest struct {
	Name           string                       `json:"name"`
	Type           string                       `json:"type"`
	Enabled        bool                         `json:"enabled"`
	StartsAt       time.Time                    `json:"starts_at"`
	EndsAt         time.Time                    `json:"ends_at"`
	MaxUsesPerUser int                          `json:"max_uses_per_user"`
	PlanBonuses    []PromotionActivityPlanInput `json:"plan_bonuses"`
}

type PromotionActivityPlanView struct {
	ID        int64 `json:"id"`
	PlanID    int64 `json:"plan_id"`
	BonusDays int   `json:"bonus_days"`
}

type PromotionActivityView struct {
	ID             int64                       `json:"id"`
	Name           string                      `json:"name"`
	Type           string                      `json:"type"`
	Enabled        bool                        `json:"enabled"`
	Status         string                      `json:"status"`
	StartsAt       time.Time                   `json:"starts_at"`
	EndsAt         time.Time                   `json:"ends_at"`
	MaxUsesPerUser int                         `json:"max_uses_per_user"`
	PlanBonuses    []PromotionActivityPlanView `json:"plan_bonuses"`
	CreatedAt      time.Time                   `json:"created_at"`
	UpdatedAt      time.Time                   `json:"updated_at"`
}

type SubscriptionBonusBenefit struct {
	ActivityID int64     `json:"activity_id"`
	Days       int       `json:"days"`
	EndsAt     time.Time `json:"ends_at"`
}

func (s *PaymentConfigService) ListPromotionActivities(ctx context.Context) ([]PromotionActivityView, error) {
	activities, err := s.entClient.PromotionActivity.Query().
		WithPlanBonuses(func(q *dbent.PromotionActivityPlanQuery) {
			q.Order(dbent.Asc(promotionactivityplan.FieldPlanID))
		}).
		Order(dbent.Desc(promotionactivity.FieldCreatedAt), dbent.Desc(promotionactivity.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list promotion activities: %w", err)
	}
	now := time.Now()
	result := make([]PromotionActivityView, 0, len(activities))
	for _, activity := range activities {
		result = append(result, promotionActivityView(activity, now))
	}
	return result, nil
}

func (s *PaymentConfigService) GetPromotionActivity(ctx context.Context, id int64) (*PromotionActivityView, error) {
	activity, err := s.entClient.PromotionActivity.Query().
		Where(promotionactivity.IDEQ(id)).
		WithPlanBonuses(func(q *dbent.PromotionActivityPlanQuery) {
			q.Order(dbent.Asc(promotionactivityplan.FieldPlanID))
		}).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("PROMOTION_ACTIVITY_NOT_FOUND", "promotion activity not found")
		}
		return nil, fmt.Errorf("get promotion activity: %w", err)
	}
	view := promotionActivityView(activity, time.Now())
	return &view, nil
}

func (s *PaymentConfigService) CreatePromotionActivity(ctx context.Context, req UpsertPromotionActivityRequest) (*PromotionActivityView, error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin promotion activity transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := lockPromotionActivityPlansForUpdate(txCtx, tx.Client(), req.PlanBonuses); err != nil {
		return nil, err
	}
	bonuses, err := s.validatePromotionActivityRequest(txCtx, tx.Client(), req, 0, false)
	if err != nil {
		return nil, err
	}
	activity, err := tx.PromotionActivity.Create().
		SetName(strings.TrimSpace(req.Name)).
		SetActivityType(req.Type).
		SetEnabled(req.Enabled).
		SetStartsAt(req.StartsAt).
		SetEndsAt(req.EndsAt).
		SetMaxUsesPerUser(req.MaxUsesPerUser).
		Save(txCtx)
	if err != nil {
		return nil, fmt.Errorf("create promotion activity: %w", err)
	}
	if err := createPromotionActivityPlans(txCtx, tx.Client(), activity.ID, bonuses); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit promotion activity: %w", err)
	}
	return s.GetPromotionActivity(ctx, activity.ID)
}

func (s *PaymentConfigService) UpdatePromotionActivity(ctx context.Context, id int64, req UpsertPromotionActivityRequest) (*PromotionActivityView, error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin promotion activity update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	currentQuery := tx.PromotionActivity.Query().
		Where(promotionactivity.IDEQ(id)).
		WithPlanBonuses()
	if supportsForUpdate(tx.Client()) {
		currentQuery = currentQuery.ForUpdate()
	}
	current, err := currentQuery.Only(txCtx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, infraerrors.NotFound("PROMOTION_ACTIVITY_NOT_FOUND", "promotion activity not found")
		}
		return nil, fmt.Errorf("get promotion activity: %w", err)
	}
	hasAnyParticipation, err := tx.PromotionActivityParticipation.Query().
		Where(promotionactivityparticipation.ActivityIDEQ(id)).
		Exist(txCtx)
	if err != nil {
		return nil, fmt.Errorf("check promotion activity participation: %w", err)
	}
	hasBlockingParticipation, err := tx.PromotionActivityParticipation.Query().
		Where(
			promotionactivityparticipation.ActivityIDEQ(id),
			promotionactivityparticipation.StatusIn(PromotionParticipationStatusReserved, PromotionParticipationStatusGranted),
		).
		Exist(txCtx)
	if err != nil {
		return nil, fmt.Errorf("check blocking promotion activity participation: %w", err)
	}
	allowDetachedActivity := hasAnyParticipation && len(current.Edges.PlanBonuses) == 0 && len(req.PlanBonuses) == 0
	planInputs := append([]PromotionActivityPlanInput(nil), req.PlanBonuses...)
	for _, existing := range current.Edges.PlanBonuses {
		planInputs = append(planInputs, PromotionActivityPlanInput{PlanID: existing.PlanID})
	}
	if err := lockPromotionActivityPlansForUpdate(txCtx, tx.Client(), planInputs); err != nil {
		return nil, err
	}
	bonuses, err := s.validatePromotionActivityRequest(txCtx, tx.Client(), req, id, allowDetachedActivity)
	if err != nil {
		return nil, err
	}
	if hasBlockingParticipation && !promotionActivityImmutableFieldsMatch(current, req, bonuses) {
		return nil, infraerrors.Conflict("PROMOTION_ACTIVITY_IMMUTABLE", "an activity with a reserved or granted participation can only change its name or enabled state")
	}
	if _, err := tx.PromotionActivity.UpdateOneID(id).
		SetName(strings.TrimSpace(req.Name)).
		SetActivityType(req.Type).
		SetEnabled(req.Enabled).
		SetStartsAt(req.StartsAt).
		SetEndsAt(req.EndsAt).
		SetMaxUsesPerUser(req.MaxUsesPerUser).
		Save(txCtx); err != nil {
		return nil, fmt.Errorf("update promotion activity: %w", err)
	}
	if !hasBlockingParticipation {
		if _, err := tx.PromotionActivityPlan.Delete().Where(promotionactivityplan.ActivityIDEQ(id)).Exec(txCtx); err != nil {
			return nil, fmt.Errorf("replace promotion activity plans: %w", err)
		}
		if err := createPromotionActivityPlans(txCtx, tx.Client(), id, bonuses); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit promotion activity update: %w", err)
	}
	return s.GetPromotionActivity(ctx, id)
}

func lockPromotionActivityPlansForUpdate(ctx context.Context, client *dbent.Client, inputs []PromotionActivityPlanInput) error {
	if client == nil || len(inputs) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(inputs))
	seen := make(map[int64]struct{}, len(inputs))
	for _, input := range inputs {
		if input.PlanID <= 0 {
			continue
		}
		if _, ok := seen[input.PlanID]; ok {
			continue
		}
		seen[input.PlanID] = struct{}{}
		ids = append(ids, input.PlanID)
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	query := client.SubscriptionPlan.Query().
		Where(subscriptionplan.IDIn(ids...)).
		Order(dbent.Asc(subscriptionplan.FieldID))
	if dbent.TxFromContext(ctx) != nil && supportsForUpdate(client) {
		query = query.ForUpdate()
	}
	if _, err := query.All(ctx); err != nil {
		return fmt.Errorf("lock promotion activity plans: %w", err)
	}
	return nil
}

func supportsForUpdate(client *dbent.Client) bool {
	return client != nil && client.Driver() != nil && client.Driver().Dialect() != dialect.SQLite
}

func (s *PaymentConfigService) DeletePromotionActivity(ctx context.Context, id int64) error {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin delete promotion activity: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	activityQuery := tx.PromotionActivity.Query().Where(promotionactivity.IDEQ(id))
	if supportsForUpdate(tx.Client()) {
		activityQuery = activityQuery.ForUpdate()
	}
	if _, err := activityQuery.Only(txCtx); err != nil {
		if dbent.IsNotFound(err) {
			return infraerrors.NotFound("PROMOTION_ACTIVITY_NOT_FOUND", "promotion activity not found")
		}
		return fmt.Errorf("lock promotion activity: %w", err)
	}
	hasParticipation, err := tx.PromotionActivityParticipation.Query().
		Where(promotionactivityparticipation.ActivityIDEQ(id)).
		Exist(txCtx)
	if err != nil {
		return fmt.Errorf("check promotion activity participation: %w", err)
	}
	if hasParticipation {
		return infraerrors.Conflict("PROMOTION_ACTIVITY_IN_USE", "an activity with participation history cannot be deleted; disable it instead")
	}
	if _, err := tx.PromotionActivityPlan.Delete().Where(promotionactivityplan.ActivityIDEQ(id)).Exec(txCtx); err != nil {
		return fmt.Errorf("delete promotion activity plans: %w", err)
	}
	if err := tx.PromotionActivity.DeleteOneID(id).Exec(txCtx); err != nil {
		return fmt.Errorf("delete promotion activity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete promotion activity: %w", err)
	}
	return nil
}

func (s *PaymentConfigService) GetEligibleSubscriptionBonuses(ctx context.Context, userID int64, planIDs []int64) (map[int64]*SubscriptionBonusBenefit, error) {
	result := make(map[int64]*SubscriptionBonusBenefit)
	if userID <= 0 || len(planIDs) == 0 {
		return result, nil
	}
	now := time.Now()
	activities, err := s.entClient.PromotionActivity.Query().
		Where(
			promotionactivity.ActivityTypeEQ(PromotionActivityTypeSubscriptionBonusDays),
			promotionactivity.EnabledEQ(true),
			promotionactivity.StartsAtLTE(now),
			promotionactivity.EndsAtGT(now),
			promotionactivity.HasPlanBonusesWith(promotionactivityplan.PlanIDIn(planIDs...)),
		).
		WithPlanBonuses(func(q *dbent.PromotionActivityPlanQuery) {
			q.Where(promotionactivityplan.PlanIDIn(planIDs...))
		}).
		Order(dbent.Asc(promotionactivity.FieldEndsAt), dbent.Asc(promotionactivity.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list eligible subscription bonus activities: %w", err)
	}
	if len(activities) == 0 {
		return result, nil
	}
	activityIDs := make([]int64, 0, len(activities))
	for _, activity := range activities {
		activityIDs = append(activityIDs, activity.ID)
	}
	participations, err := s.entClient.PromotionActivityParticipation.Query().
		Where(
			promotionactivityparticipation.UserIDEQ(userID),
			promotionactivityparticipation.ActivityIDIn(activityIDs...),
			promotionactivityparticipation.StatusIn(PromotionParticipationStatusReserved, PromotionParticipationStatusGranted),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list promotion activity participations: %w", err)
	}
	used := make(map[int64]int, len(activityIDs))
	for _, participation := range participations {
		used[participation.ActivityID]++
	}
	for _, activity := range activities {
		if used[activity.ID] >= activity.MaxUsesPerUser {
			continue
		}
		for _, bonus := range activity.Edges.PlanBonuses {
			if _, exists := result[bonus.PlanID]; exists {
				continue
			}
			result[bonus.PlanID] = &SubscriptionBonusBenefit{ActivityID: activity.ID, Days: bonus.BonusDays, EndsAt: activity.EndsAt}
		}
	}
	return result, nil
}

func (s *PaymentConfigService) validatePromotionActivityRequest(ctx context.Context, client *dbent.Client, req UpsertPromotionActivityRequest, excludeID int64, allowEmptyPlans bool) ([]PromotionActivityPlanInput, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return nil, infraerrors.BadRequest("PROMOTION_ACTIVITY_NAME_REQUIRED", "activity name is required")
	}
	if len([]rune(req.Name)) > 100 {
		return nil, infraerrors.BadRequest("PROMOTION_ACTIVITY_NAME_TOO_LONG", "activity name must be at most 100 characters")
	}
	if req.Type != PromotionActivityTypeSubscriptionBonusDays {
		return nil, infraerrors.BadRequest("PROMOTION_ACTIVITY_TYPE_INVALID", "unsupported promotion activity type")
	}
	if req.StartsAt.IsZero() || req.EndsAt.IsZero() || !req.EndsAt.After(req.StartsAt) {
		return nil, infraerrors.BadRequest("PROMOTION_ACTIVITY_WINDOW_INVALID", "activity end time must be later than start time")
	}
	if req.MaxUsesPerUser <= 0 || req.MaxUsesPerUser > 1000 {
		return nil, infraerrors.BadRequest("PROMOTION_ACTIVITY_MAX_USES_INVALID", "max uses per user must be between 1 and 1000")
	}
	if len(req.PlanBonuses) == 0 {
		if allowEmptyPlans {
			return nil, nil
		}
		return nil, infraerrors.BadRequest("PROMOTION_ACTIVITY_PLANS_REQUIRED", "at least one subscription plan is required")
	}
	bonuses := append([]PromotionActivityPlanInput(nil), req.PlanBonuses...)
	sort.Slice(bonuses, func(i, j int) bool { return bonuses[i].PlanID < bonuses[j].PlanID })
	planIDs := make([]int64, 0, len(bonuses))
	seen := make(map[int64]struct{}, len(bonuses))
	for _, bonus := range bonuses {
		if bonus.PlanID <= 0 || bonus.BonusDays <= 0 || bonus.BonusDays > MaxValidityDays {
			return nil, infraerrors.BadRequest("PROMOTION_ACTIVITY_PLAN_INVALID", "plan and bonus days must be valid")
		}
		if _, exists := seen[bonus.PlanID]; exists {
			return nil, infraerrors.BadRequest("PROMOTION_ACTIVITY_PLAN_DUPLICATE", "subscription plan is duplicated")
		}
		seen[bonus.PlanID] = struct{}{}
		planIDs = append(planIDs, bonus.PlanID)
	}
	plans, err := client.SubscriptionPlan.Query().Where(subscriptionplan.IDIn(planIDs...)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("validate promotion activity plans: %w", err)
	}
	if len(plans) != len(planIDs) {
		return nil, infraerrors.BadRequest("PROMOTION_ACTIVITY_PLAN_NOT_FOUND", "one or more subscription plans do not exist")
	}
	bonusByPlan := make(map[int64]int, len(bonuses))
	for _, bonus := range bonuses {
		bonusByPlan[bonus.PlanID] = bonus.BonusDays
	}
	for _, plan := range plans {
		baseDays, validityErr := validateSubscriptionPlanValidity(plan.ValidityDays, plan.ValidityUnit)
		if validityErr != nil || bonusByPlan[plan.ID] > MaxValidityDays-baseDays {
			return nil, infraerrors.BadRequest("PROMOTION_ACTIVITY_VALIDITY_TOO_LONG", "plan validity plus bonus days exceeds the maximum subscription validity")
		}
	}
	if req.Enabled {
		activityPredicates := []predicate.PromotionActivity{
			promotionactivity.ActivityTypeEQ(req.Type),
			promotionactivity.EnabledEQ(true),
			promotionactivity.StartsAtLT(req.EndsAt),
			promotionactivity.EndsAtGT(req.StartsAt),
		}
		if excludeID > 0 {
			activityPredicates = append(activityPredicates, promotionactivity.IDNEQ(excludeID))
		}
		overlap, err := client.PromotionActivityPlan.Query().Where(
			promotionactivityplan.PlanIDIn(planIDs...),
			promotionactivityplan.HasActivityWith(activityPredicates...),
		).Exist(ctx)
		if err != nil {
			return nil, fmt.Errorf("check promotion activity overlap: %w", err)
		}
		if overlap {
			return nil, infraerrors.Conflict("PROMOTION_ACTIVITY_OVERLAP", "an enabled activity of the same type overlaps for one or more plans")
		}
	}
	return bonuses, nil
}

func createPromotionActivityPlans(ctx context.Context, client *dbent.Client, activityID int64, bonuses []PromotionActivityPlanInput) error {
	builders := make([]*dbent.PromotionActivityPlanCreate, 0, len(bonuses))
	for _, bonus := range bonuses {
		builders = append(builders, client.PromotionActivityPlan.Create().
			SetActivityID(activityID).
			SetPlanID(bonus.PlanID).
			SetBonusDays(bonus.BonusDays))
	}
	if _, err := client.PromotionActivityPlan.CreateBulk(builders...).Save(ctx); err != nil {
		return fmt.Errorf("create promotion activity plans: %w", err)
	}
	return nil
}

func promotionActivityView(activity *dbent.PromotionActivity, now time.Time) PromotionActivityView {
	planBonuses := make([]PromotionActivityPlanView, 0, len(activity.Edges.PlanBonuses))
	for _, bonus := range activity.Edges.PlanBonuses {
		planBonuses = append(planBonuses, PromotionActivityPlanView{ID: bonus.ID, PlanID: bonus.PlanID, BonusDays: bonus.BonusDays})
	}
	return PromotionActivityView{
		ID: activity.ID, Name: activity.Name, Type: activity.ActivityType, Enabled: activity.Enabled,
		Status: promotionActivityStatus(activity, now), StartsAt: activity.StartsAt, EndsAt: activity.EndsAt,
		MaxUsesPerUser: activity.MaxUsesPerUser, PlanBonuses: planBonuses,
		CreatedAt: activity.CreatedAt, UpdatedAt: activity.UpdatedAt,
	}
}

func promotionActivityStatus(activity *dbent.PromotionActivity, now time.Time) string {
	if !activity.Enabled {
		return PromotionActivityStatusDisabled
	}
	if now.Before(activity.StartsAt) {
		return PromotionActivityStatusScheduled
	}
	if !now.Before(activity.EndsAt) {
		return PromotionActivityStatusEnded
	}
	return PromotionActivityStatusActive
}

func promotionActivityImmutableFieldsMatch(activity *dbent.PromotionActivity, req UpsertPromotionActivityRequest, bonuses []PromotionActivityPlanInput) bool {
	if activity.ActivityType != req.Type || !activity.StartsAt.Equal(req.StartsAt) || !activity.EndsAt.Equal(req.EndsAt) || activity.MaxUsesPerUser != req.MaxUsesPerUser {
		return false
	}
	if len(activity.Edges.PlanBonuses) != len(bonuses) {
		return false
	}
	existing := make(map[int64]int, len(activity.Edges.PlanBonuses))
	for _, bonus := range activity.Edges.PlanBonuses {
		existing[bonus.PlanID] = bonus.BonusDays
	}
	for _, bonus := range bonuses {
		if existing[bonus.PlanID] != bonus.BonusDays {
			return false
		}
	}
	return true
}
