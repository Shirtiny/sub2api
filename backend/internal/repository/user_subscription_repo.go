package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type userSubscriptionRepository struct {
	client *dbent.Client
}

var subscriptionNoExpirySentinel = time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)

func activeSubscriptionExpiresAt(now time.Time) predicate.UserSubscription {
	return usersubscription.Or(
		predicate.UserSubscription(sql.FieldIsNull(usersubscription.FieldExpiresAt)),
		usersubscription.ExpiresAtGT(now),
	)
}

func normalizeSubscriptionExpiresAt(expiresAt time.Time) time.Time {
	// Some legacy production rows can have NULL expires_at even though the current
	// ent schema is non-nullable. ent scans those rows as the zero time; normalize
	// them to the existing long-lived sentinel so downstream service checks do not
	// treat a row returned by activeSubscriptionExpiresAt as already expired.
	if expiresAt.IsZero() {
		return subscriptionNoExpirySentinel
	}
	return expiresAt
}

func NewUserSubscriptionRepository(client *dbent.Client) service.UserSubscriptionRepository {
	return &userSubscriptionRepository{client: client}
}

func (r *userSubscriptionRepository) Create(ctx context.Context, sub *service.UserSubscription) error {
	if sub == nil {
		return service.ErrSubscriptionNilInput
	}

	client := clientFromContext(ctx, r.client)
	builder := client.UserSubscription.Create().
		SetUserID(sub.UserID).
		SetGroupID(sub.GroupID).
		SetExpiresAt(sub.ExpiresAt).
		SetNillableDailyWindowStart(sub.DailyWindowStart).
		SetNillableWeeklyWindowStart(sub.WeeklyWindowStart).
		SetNillableMonthlyWindowStart(sub.MonthlyWindowStart).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetResetCount(sub.ResetCount).
		SetEarlyResetEnabled(sub.EarlyResetEnabled).
		SetEarlyResetDurationDays(sub.EarlyResetDurationDays).
		SetNillableAssignedBy(sub.AssignedBy).
		SetNillablePlanConcurrency(sub.PlanConcurrency).
		SetNillablePlanConcurrencyExpiresAt(sub.PlanConcurrencyExpiresAt).
		SetNillableCustomMultiplier(sub.CustomMultiplier).
		SetNillableCustomSourcePlanID(sub.CustomSourcePlanID).
		SetNillableCustomSourceGroupID(sub.CustomSourceGroupID).
		SetNillableCustomExpiresAt(sub.CustomExpiresAt).
		SetNillableCustomDisplayName(nilIfEmptyString(sub.CustomDisplayName))

	if sub.StartsAt.IsZero() {
		builder.SetStartsAt(time.Now())
	} else {
		builder.SetStartsAt(sub.StartsAt)
	}
	if sub.Status != "" {
		builder.SetStatus(sub.Status)
	}
	if !sub.AssignedAt.IsZero() {
		builder.SetAssignedAt(sub.AssignedAt)
	}
	// Keep compatibility with historical behavior: always store notes as a string value.
	builder.SetNotes(sub.Notes)

	created, err := builder.Save(ctx)
	if err == nil {
		applyUserSubscriptionEntityToService(sub, created)
	}
	return translatePersistenceError(err, nil, service.ErrSubscriptionAlreadyExists)
}

func (r *userSubscriptionRepository) GetByID(ctx context.Context, id int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Query().
		Where(usersubscription.IDEQ(id)).
		WithUser().
		WithGroup().
		WithAssignedByUser().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(m), nil
}

func (r *userSubscriptionRepository) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID), usersubscription.GroupIDEQ(groupID)).
		WithGroup().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(m), nil
}

func (r *userSubscriptionRepository) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(userID),
			usersubscription.GroupIDEQ(groupID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			activeSubscriptionExpiresAt(time.Now()),
		).
		WithGroup().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(m), nil
}

func (r *userSubscriptionRepository) Update(ctx context.Context, sub *service.UserSubscription) error {
	if sub == nil {
		return service.ErrSubscriptionNilInput
	}

	client := clientFromContext(ctx, r.client)
	builder := client.UserSubscription.UpdateOneID(sub.ID).
		SetUserID(sub.UserID).
		SetGroupID(sub.GroupID).
		SetStartsAt(sub.StartsAt).
		SetExpiresAt(sub.ExpiresAt).
		SetStatus(sub.Status).
		SetNillableDailyWindowStart(sub.DailyWindowStart).
		SetNillableWeeklyWindowStart(sub.WeeklyWindowStart).
		SetNillableMonthlyWindowStart(sub.MonthlyWindowStart).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetResetCount(sub.ResetCount).
		SetEarlyResetEnabled(sub.EarlyResetEnabled).
		SetEarlyResetDurationDays(sub.EarlyResetDurationDays).
		SetNillableAssignedBy(sub.AssignedBy).
		SetAssignedAt(sub.AssignedAt).
		SetNotes(sub.Notes)

	if sub.PlanConcurrency != nil {
		builder = builder.SetPlanConcurrency(*sub.PlanConcurrency)
	} else {
		builder = builder.ClearPlanConcurrency()
	}
	if sub.PlanConcurrencyExpiresAt != nil {
		builder = builder.SetPlanConcurrencyExpiresAt(*sub.PlanConcurrencyExpiresAt)
	} else {
		builder = builder.ClearPlanConcurrencyExpiresAt()
	}

	if sub.CustomMultiplier != nil {
		builder = builder.SetCustomMultiplier(*sub.CustomMultiplier)
	} else {
		builder = builder.ClearCustomMultiplier()
	}
	if sub.CustomSourcePlanID != nil {
		builder = builder.SetCustomSourcePlanID(*sub.CustomSourcePlanID)
	} else {
		builder = builder.ClearCustomSourcePlanID()
	}
	if sub.CustomSourceGroupID != nil {
		builder = builder.SetCustomSourceGroupID(*sub.CustomSourceGroupID)
	} else {
		builder = builder.ClearCustomSourceGroupID()
	}
	if sub.CustomExpiresAt != nil {
		builder = builder.SetCustomExpiresAt(*sub.CustomExpiresAt)
	} else {
		builder = builder.ClearCustomExpiresAt()
	}
	if sub.CustomDisplayName != "" {
		builder = builder.SetCustomDisplayName(sub.CustomDisplayName)
	} else {
		builder = builder.ClearCustomDisplayName()
	}

	updated, err := builder.Save(ctx)
	if err == nil {
		applyUserSubscriptionEntityToService(sub, updated)
		return nil
	}
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, service.ErrSubscriptionAlreadyExists)
}

func (r *userSubscriptionRepository) Delete(ctx context.Context, id int64) error {
	// Match GORM semantics: deleting a missing row is not an error.
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.Delete().Where(usersubscription.IDEQ(id)).Exec(ctx)
	return err
}

func (r *userSubscriptionRepository) ListByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	subs, err := client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID)).
		WithGroup().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return userSubscriptionEntitiesToService(subs), nil
}

func (r *userSubscriptionRepository) ListActiveByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	subs, err := client.UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(userID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			activeSubscriptionExpiresAt(time.Now()),
		).
		WithGroup().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return userSubscriptionEntitiesToService(subs), nil
}

func (r *userSubscriptionRepository) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := client.UserSubscription.Query().Where(usersubscription.GroupIDEQ(groupID))

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	subs, err := q.
		WithUser().
		WithGroup().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	return userSubscriptionEntitiesToService(subs), paginationResultFromTotal(int64(total), params), nil
}

func (r *userSubscriptionRepository) List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status, platform, sortBy, sortOrder string) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := client.UserSubscription.Query()
	if userID != nil {
		q = q.Where(usersubscription.UserIDEQ(*userID))
	}
	if groupID != nil {
		q = q.Where(usersubscription.GroupIDEQ(*groupID))
	}
	if platform != "" {
		q = q.Where(usersubscription.HasGroupWith(group.PlatformEQ(platform)))
	}

	// Status filtering with real-time expiration check
	now := time.Now()
	switch status {
	case service.SubscriptionStatusActive:
		// Active: status is active AND not yet expired
		q = q.Where(
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			activeSubscriptionExpiresAt(now),
		)
	case service.SubscriptionStatusExpired:
		// Expired: status is expired OR (status is active but already expired)
		q = q.Where(
			usersubscription.Or(
				usersubscription.StatusEQ(service.SubscriptionStatusExpired),
				usersubscription.And(
					usersubscription.StatusEQ(service.SubscriptionStatusActive),
					usersubscription.ExpiresAtLTE(now),
				),
			),
		)
	case "":
		// No filter
	default:
		// Other status (e.g., revoked)
		q = q.Where(usersubscription.StatusEQ(status))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Apply sorting
	q = q.WithUser().WithGroup().WithAssignedByUser()

	// Determine sort field
	var field string
	switch sortBy {
	case "expires_at":
		field = usersubscription.FieldExpiresAt
	case "status":
		field = usersubscription.FieldStatus
	default:
		field = usersubscription.FieldCreatedAt
	}

	// Determine sort order (default: desc)
	if sortOrder == "asc" && sortBy != "" {
		q = q.Order(dbent.Asc(field))
	} else {
		q = q.Order(dbent.Desc(field))
	}

	subs, err := q.
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	return userSubscriptionEntitiesToService(subs), paginationResultFromTotal(int64(total), params), nil
}

func (r *userSubscriptionRepository) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	client := clientFromContext(ctx, r.client)
	return client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID), usersubscription.GroupIDEQ(groupID)).
		Exist(ctx)
}

func (r *userSubscriptionRepository) ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(subscriptionID).
		SetExpiresAt(newExpiresAt).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) UpdateStatus(ctx context.Context, subscriptionID int64, status string) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(subscriptionID).
		SetStatus(status).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(subscriptionID).
		SetNotes(notes).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) ActivateWindows(ctx context.Context, id int64, start time.Time) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(id).
		SetDailyWindowStart(start).
		SetWeeklyWindowStart(start).
		SetMonthlyWindowStart(start).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) ResetUsageWindows(ctx context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	update := client.UserSubscription.UpdateOneID(id)
	if resetDaily {
		update.SetDailyUsageUsd(0).SetDailyWindowStart(newWindowStart)
	}
	if resetWeekly {
		update.SetWeeklyUsageUsd(0).SetWeeklyWindowStart(newWindowStart)
	}
	if resetMonthly {
		update.SetMonthlyUsageUsd(0).SetMonthlyWindowStart(newWindowStart)
	}
	_, err := update.Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

// ResetUserQuota atomically consumes one user reset allowance and resets the
// daily and weekly windows. Ownership, active status, expiry, the
// early-reset exclusion (for the default/direct path) and the remaining-count
// check all live in the conditional UPDATE; the service-authoritative variant
// can replace only the stale policy snapshot after resolving entitlements.
func (r *userSubscriptionRepository) ResetUserQuota(ctx context.Context, params service.UserQuotaResetParams) (*service.UserSubscription, error) {
	return r.ResetUserQuotaWithPolicy(ctx, params, false)
}

// ResetUserQuotaWithPolicy is used by the service after it has resolved the
// current entitlement term. The ordinary method above retains a persisted
// early-reset guard for direct callers.
func (r *userSubscriptionRepository) ResetUserQuotaWithPolicy(ctx context.Context, params service.UserQuotaResetParams, skipPersistedEarlyResetPolicy bool) (*service.UserSubscription, error) {
	if params.SubscriptionID <= 0 || params.UserID <= 0 {
		return nil, service.ErrSubscriptionNotFound
	}
	now := params.Now
	if now.IsZero() {
		now = time.Now()
	}
	windowStart := params.WindowStart
	if windowStart.IsZero() {
		windowStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}

	client := clientFromContext(ctx, r.client)
	update := client.UserSubscription.Update().Where(
		usersubscription.IDEQ(params.SubscriptionID),
		usersubscription.UserIDEQ(params.UserID),
		usersubscription.DeletedAtIsNil(),
		usersubscription.StatusEQ(service.SubscriptionStatusActive),
		usersubscription.StartsAtLTE(now),
		activeSubscriptionExpiresAt(now),
		usersubscription.ResetCountGT(0),
	)
	if !skipPersistedEarlyResetPolicy {
		update = update.Where(
			usersubscription.EarlyResetEnabledEQ(false),
			usersubscription.EarlyResetDurationDaysEQ(0),
		)
	}
	updated, err := update.
		AddResetCount(-1).
		SetDailyUsageUsd(0).
		SetWeeklyUsageUsd(0).
		SetDailyWindowStart(windowStart).
		SetWeeklyWindowStart(windowStart).
		Save(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	if updated == 0 {
		return nil, r.classifyUserQuotaResetMiss(ctx, client, params.SubscriptionID, params.UserID, now, skipPersistedEarlyResetPolicy)
	}

	// Read the row through the same client/transaction so callers receive the
	// post-decrement count and the normal user/group associations.  The service
	// converts it before the surrounding transaction is committed.
	m, err := client.UserSubscription.Query().
		Where(usersubscription.IDEQ(params.SubscriptionID)).
		WithUser().
		WithGroup().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(m), nil
}

// classifyUserQuotaResetMiss turns a conditional-update miss into a stable
// business error without revealing whether an arbitrary subscription belongs
// to another user.  The initial lookup in the service performs the same
// ownership check for the normal path; this second check is for races.
func (r *userSubscriptionRepository) classifyUserQuotaResetMiss(ctx context.Context, client *dbent.Client, id, userID int64, now time.Time, skipPersistedEarlyResetPolicy bool) error {
	m, err := client.UserSubscription.Query().Where(usersubscription.IDEQ(id)).Only(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	if m.UserID != userID {
		return service.ErrSubscriptionNotFound
	}
	if m.Status == service.SubscriptionStatusSuspended {
		return service.ErrSubscriptionSuspended
	}
	if m.Status == "revoked" {
		return service.ErrSubscriptionNotFound
	}
	if m.StartsAt.After(now) {
		return service.ErrSubscriptionNotStarted
	}
	if m.Status != service.SubscriptionStatusActive || !normalizeSubscriptionExpiresAt(m.ExpiresAt).After(now) {
		return service.ErrSubscriptionExpired
	}
	if !skipPersistedEarlyResetPolicy && (m.EarlyResetEnabled || m.EarlyResetDurationDays != 0) {
		return service.ErrQuotaResetDisabled
	}
	if m.ResetCount <= 0 {
		return service.ErrQuotaResetExhausted
	}
	return service.ErrQuotaResetConflict
}

// SetResetCount changes the remaining user reset allowance for one
// subscription.  Positive allowances are never written to a subscription
// carrying the one-time/early-reset policy.
func (r *userSubscriptionRepository) SetResetCount(ctx context.Context, subscriptionID int64, count int) error {
	return r.SetResetCountWithPolicy(ctx, subscriptionID, count, false)
}

// SetResetCountWithPolicy is the policy-aware variant used by the service
// after it has resolved the current entitlement term. The default method
// above keeps the persisted early-reset guard for direct callers.
func (r *userSubscriptionRepository) SetResetCountWithPolicy(ctx context.Context, subscriptionID int64, count int, skipPersistedEarlyResetPolicy bool) error {
	if subscriptionID <= 0 {
		return service.ErrSubscriptionNotFound
	}
	if count < 0 || count > service.MaxUserQuotaResetCount {
		return service.ErrInvalidQuotaResetCount
	}
	client := clientFromContext(ctx, r.client)
	update := client.UserSubscription.Update().Where(
		usersubscription.IDEQ(subscriptionID),
		usersubscription.DeletedAtIsNil(),
		usersubscription.StatusNEQ("revoked"),
	)
	if count > 0 && !skipPersistedEarlyResetPolicy {
		update = update.Where(
			usersubscription.EarlyResetEnabledEQ(false),
			usersubscription.EarlyResetDurationDaysEQ(0),
		)
	}
	affected, err := update.SetResetCount(count).Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	if affected > 0 {
		return nil
	}
	if count > 0 {
		m, lookupErr := client.UserSubscription.Query().Where(usersubscription.IDEQ(subscriptionID)).Only(ctx)
		if lookupErr != nil {
			return translatePersistenceError(lookupErr, service.ErrSubscriptionNotFound, nil)
		}
		if m.Status == "revoked" {
			return service.ErrSubscriptionNotFound
		}
		if m.EarlyResetEnabled || m.EarlyResetDurationDays != 0 {
			return service.ErrQuotaResetDisabled
		}
		return service.ErrQuotaResetConflict
	}
	return service.ErrSubscriptionNotFound
}

// BulkSetResetCount updates a bounded set of subscriptions in one statement.
// The service filters current early-reset terms before calling this method;
// the predicate below also protects against the persisted policy snapshot.
func (r *userSubscriptionRepository) BulkSetResetCount(ctx context.Context, subscriptionIDs []int64, count int) (int64, error) {
	return r.BulkSetResetCountWithPolicy(ctx, subscriptionIDs, count, false)
}

// BulkSetResetCountWithPolicy is the policy-aware variant used after the
// service has resolved current entitlement snapshots for the selected rows.
func (r *userSubscriptionRepository) BulkSetResetCountWithPolicy(ctx context.Context, subscriptionIDs []int64, count int, skipPersistedEarlyResetPolicy bool) (int64, error) {
	if count < 0 || count > service.MaxUserQuotaResetCount {
		return 0, service.ErrInvalidQuotaResetCount
	}
	if len(subscriptionIDs) > 1000 {
		return 0, infraerrors.BadRequest("TOO_MANY_SUBSCRIPTIONS", "at most 1000 subscriptions may be updated at once")
	}
	if len(subscriptionIDs) == 0 {
		return 0, nil
	}
	client := clientFromContext(ctx, r.client)
	update := client.UserSubscription.Update().Where(
		usersubscription.IDIn(subscriptionIDs...),
		usersubscription.DeletedAtIsNil(),
		usersubscription.StatusNEQ("revoked"),
	)
	if count > 0 && !skipPersistedEarlyResetPolicy {
		update = update.Where(
			usersubscription.EarlyResetEnabledEQ(false),
			usersubscription.EarlyResetDurationDaysEQ(0),
		)
	}
	affected, err := update.SetResetCount(count).Save(ctx)
	return int64(affected), translatePersistenceError(err, nil, nil)
}

func (r *userSubscriptionRepository) EarlyReset(ctx context.Context, input service.EarlyResetSubscriptionParams) error {
	client := clientFromContext(ctx, r.client)
	update := client.UserSubscription.Update().Where(
		usersubscription.IDEQ(input.ID),
		usersubscription.UserIDEQ(input.UserID),
		usersubscription.StatusEQ(service.SubscriptionStatusActive),
		usersubscription.ExpiresAtEQ(input.ExpectedExpiresAt),
	)
	if input.ExpectedCustomExpiresAt == nil {
		update = update.Where(usersubscription.CustomExpiresAtIsNil())
	} else {
		update = update.Where(usersubscription.CustomExpiresAtEQ(*input.ExpectedCustomExpiresAt))
	}
	update = update.
		SetExpiresAt(input.NewExpiresAt).
		SetDailyUsageUsd(0).
		SetWeeklyUsageUsd(0).
		SetMonthlyUsageUsd(0).
		SetDailyWindowStart(input.WindowStart).
		SetWeeklyWindowStart(input.WindowStart).
		SetMonthlyWindowStart(input.WindowStart)
	if input.NewCustomExpiresAt == nil {
		update = update.ClearCustomExpiresAt()
	} else {
		update = update.SetCustomExpiresAt(*input.NewCustomExpiresAt)
	}
	affected, err := update.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	if affected > 0 {
		return nil
	}
	exists, err := client.UserSubscription.Query().Where(usersubscription.IDEQ(input.ID)).Exist(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	if !exists {
		return service.ErrSubscriptionNotFound
	}
	return service.ErrEarlyResetConflict
}

func (r *userSubscriptionRepository) ResetDailyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	query := client.UserSubscription.Update().Where(usersubscription.IDEQ(id))
	if expectedWindowStart == nil {
		query = query.Where(usersubscription.DailyWindowStartIsNil())
	} else {
		query = query.Where(usersubscription.DailyWindowStartEQ(*expectedWindowStart))
	}
	n, err := query.
		SetDailyUsageUsd(0).
		SetDailyWindowStart(newWindowStart).
		Save(ctx)
	return r.translateConditionalWindowReset(ctx, client, id, n, err)
}

func (r *userSubscriptionRepository) ResetWeeklyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	query := client.UserSubscription.Update().Where(usersubscription.IDEQ(id))
	if expectedWindowStart == nil {
		query = query.Where(usersubscription.WeeklyWindowStartIsNil())
	} else {
		query = query.Where(usersubscription.WeeklyWindowStartEQ(*expectedWindowStart))
	}
	n, err := query.
		SetWeeklyUsageUsd(0).
		SetWeeklyWindowStart(newWindowStart).
		Save(ctx)
	return r.translateConditionalWindowReset(ctx, client, id, n, err)
}

func (r *userSubscriptionRepository) ResetMonthlyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	query := client.UserSubscription.Update().Where(usersubscription.IDEQ(id))
	if expectedWindowStart == nil {
		query = query.Where(usersubscription.MonthlyWindowStartIsNil())
	} else {
		query = query.Where(usersubscription.MonthlyWindowStartEQ(*expectedWindowStart))
	}
	n, err := query.
		SetMonthlyUsageUsd(0).
		SetMonthlyWindowStart(newWindowStart).
		Save(ctx)
	return r.translateConditionalWindowReset(ctx, client, id, n, err)
}

func (r *userSubscriptionRepository) translateConditionalWindowReset(ctx context.Context, client *dbent.Client, id int64, affected int, err error) error {
	if err != nil {
		return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	if affected > 0 {
		return nil
	}

	// A stale reset is an expected no-op: another request already advanced the
	// window. Preserve not-found semantics for callers that target a missing row.
	exists, err := client.UserSubscription.Query().Where(usersubscription.IDEQ(id)).Exist(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	if !exists {
		return service.ErrSubscriptionNotFound
	}
	return nil
}

func (r *userSubscriptionRepository) ResetActiveUsage(ctx context.Context, resetDaily, resetWeekly, resetMonthly bool, newWindowStart time.Time) (int64, error) {
	client := clientFromContext(ctx, r.client)
	update := client.UserSubscription.Update().
		Where(
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			activeSubscriptionExpiresAt(time.Now()),
		)
	if resetDaily {
		update.SetDailyUsageUsd(0).SetDailyWindowStart(newWindowStart)
	}
	if resetWeekly {
		update.SetWeeklyUsageUsd(0).SetWeeklyWindowStart(newWindowStart)
	}
	if resetMonthly {
		update.SetMonthlyUsageUsd(0).SetMonthlyWindowStart(newWindowStart)
	}
	count, err := update.Save(ctx)
	return int64(count), translatePersistenceError(err, nil, nil)
}

// ShiftUsageWindows 按过滤条件批量平移订阅的窗口起点，只改 *_window_start，不动任何 *_usage_usd。
//
// 三条不变量：
//   - 只命中「窗口起点非 NULL」且「所属分组确实设了该窗口限额」的行，其余行完全不碰；
//   - 平移后任一目标窗口起点会落到未来的行整行跳过（Future=true），避免造出比一个周期更长的窗口；
//   - dryRun 时 UPDATE 分支恒不命中，仅返回命中明细供前端预览。
func (r *userSubscriptionRepository) ShiftUsageWindows(ctx context.Context, input service.ShiftWindowQuery) (service.ShiftWindowRows, error) {
	var result service.ShiftWindowRows
	if !input.Daily && !input.Weekly && !input.Monthly {
		return result, nil
	}
	if input.Offset == 0 {
		return result, nil
	}

	conds := []string{"us.deleted_at IS NULL", "g.deleted_at IS NULL"}
	args := []any{
		input.Daily,
		input.Weekly,
		input.Monthly,
		input.Offset.Hours(),
		input.DryRun,
	}
	now := time.Now()
	switch input.Status {
	case service.SubscriptionStatusActive:
		args = append(args, now)
		conds = append(conds, fmt.Sprintf("us.status = '%s'", service.SubscriptionStatusActive))
		conds = append(conds, fmt.Sprintf("(us.expires_at IS NULL OR us.expires_at > $%d)", len(args)))
	case service.SubscriptionStatusExpired:
		args = append(args, now)
		conds = append(conds, fmt.Sprintf("(us.status = '%s' OR (us.status = '%s' AND us.expires_at <= $%d))",
			service.SubscriptionStatusExpired, service.SubscriptionStatusActive, len(args)))
	case "":
		// 不过滤状态
	default:
		args = append(args, input.Status)
		conds = append(conds, fmt.Sprintf("us.status = $%d", len(args)))
	}
	if input.UserID != nil {
		args = append(args, *input.UserID)
		conds = append(conds, fmt.Sprintf("us.user_id = $%d", len(args)))
	}
	if input.GroupID != nil {
		args = append(args, *input.GroupID)
		conds = append(conds, fmt.Sprintf("us.group_id = $%d", len(args)))
	}
	if input.Platform != "" {
		args = append(args, input.Platform)
		conds = append(conds, fmt.Sprintf("g.platform = $%d", len(args)))
	}

	query := fmt.Sprintf(`
		WITH scoped AS (
			SELECT
				us.id,
				us.user_id,
				us.group_id,
				us.daily_window_start AS d,
				us.weekly_window_start AS w,
				us.monthly_window_start AS m,
				($1 AND g.daily_limit_usd > 0 AND us.daily_window_start IS NOT NULL) AS hit_daily,
				($2 AND g.weekly_limit_usd > 0 AND us.weekly_window_start IS NOT NULL) AS hit_weekly,
				($3 AND g.monthly_limit_usd > 0 AND us.monthly_window_start IS NOT NULL) AS hit_monthly
			FROM user_subscriptions us
			JOIN groups g ON g.id = us.group_id
			WHERE %s
		),
		matched AS (
			SELECT
				scoped.*,
				(
					(hit_daily AND d + ($4 * INTERVAL '1 hour') > NOW())
					OR (hit_weekly AND w + ($4 * INTERVAL '1 hour') > NOW())
					OR (hit_monthly AND m + ($4 * INTERVAL '1 hour') > NOW())
				) AS future
			FROM scoped
			WHERE hit_daily OR hit_weekly OR hit_monthly
		),
		upd AS (
			UPDATE user_subscriptions us
			SET
				daily_window_start = CASE WHEN matched.hit_daily
					THEN us.daily_window_start + ($4 * INTERVAL '1 hour') ELSE us.daily_window_start END,
				weekly_window_start = CASE WHEN matched.hit_weekly
					THEN us.weekly_window_start + ($4 * INTERVAL '1 hour') ELSE us.weekly_window_start END,
				monthly_window_start = CASE WHEN matched.hit_monthly
					THEN us.monthly_window_start + ($4 * INTERVAL '1 hour') ELSE us.monthly_window_start END,
				updated_at = NOW()
			FROM matched
			WHERE us.id = matched.id
				AND matched.future = FALSE
				AND $5 = FALSE
			RETURNING us.id
		)
		SELECT
			matched.id,
			matched.user_id,
			matched.group_id,
			matched.future,
			(SELECT COUNT(*) FROM upd) AS updated_count
		FROM matched
		ORDER BY matched.id
	`, strings.Join(conds, " AND "))

	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return result, translatePersistenceError(err, nil, nil)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var row service.ShiftWindowRow
		var updated int64
		if err := rows.Scan(&row.ID, &row.UserID, &row.GroupID, &row.Future, &updated); err != nil {
			return service.ShiftWindowRows{}, translatePersistenceError(err, nil, nil)
		}
		result.Rows = append(result.Rows, row)
		result.Updated = updated
	}
	if err := rows.Err(); err != nil {
		return service.ShiftWindowRows{}, translatePersistenceError(err, nil, nil)
	}
	return result, nil
}

// ListUsageDaily 读取某个订阅在 [from, to] 区间内的每日用量汇总。
// 数据来自 subscription_usage_daily（由 DashboardAggregationService 增量写入），
// 与 usage_logs 的保留期解耦，因此能覆盖整个订阅周期。
func (r *userSubscriptionRepository) ListUsageDaily(ctx context.Context, subscriptionID int64, from, to time.Time) ([]service.SubscriptionUsageDaily, error) {
	const query = `
		SELECT
			bucket_date,
			cost_usd,
			request_count,
			daily_limit_usd,
			weekly_limit_usd,
			monthly_limit_usd
		FROM subscription_usage_daily
		WHERE subscription_id = $1
			AND bucket_date >= $2::date
			AND bucket_date <= $3::date
		ORDER BY bucket_date
	`
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, query, subscriptionID, from, to)
	if err != nil {
		return nil, translatePersistenceError(err, nil, nil)
	}
	defer func() { _ = rows.Close() }()

	var out []service.SubscriptionUsageDaily
	for rows.Next() {
		var row service.SubscriptionUsageDaily
		if err := rows.Scan(
			&row.BucketDate,
			&row.CostUSD,
			&row.RequestCount,
			&row.DailyLimitUSD,
			&row.WeeklyLimitUSD,
			&row.MonthlyLimitUSD,
		); err != nil {
			return nil, translatePersistenceError(err, nil, nil)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, translatePersistenceError(err, nil, nil)
	}
	return out, nil
}

// IncrementUsage 原子性地累加订阅用量。
// 限额检查已在请求前由 BillingCacheService.CheckBillingEligibility 完成，
// 此处仅负责记录实际消费，确保消费数据的完整性。
func (r *userSubscriptionRepository) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	const updateSQL = `
		UPDATE user_subscriptions us
		SET
			daily_usage_usd = us.daily_usage_usd + $1,
			weekly_usage_usd = us.weekly_usage_usd + $1,
			monthly_usage_usd = us.monthly_usage_usd + $1,
			updated_at = NOW()
		FROM groups g
		WHERE us.id = $2
			AND us.deleted_at IS NULL
			AND us.group_id = g.id
			AND g.deleted_at IS NULL
	`

	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(ctx, updateSQL, costUSD, id)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if affected > 0 {
		return nil
	}

	// affected == 0：订阅不存在或已删除
	return service.ErrSubscriptionNotFound
}

func (r *userSubscriptionRepository) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	client := clientFromContext(ctx, r.client)
	n, err := client.UserSubscription.Update().
		Where(
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtLTE(time.Now()),
		).
		SetStatus(service.SubscriptionStatusExpired).
		Save(ctx)
	return int64(n), err
}

// Extra repository helpers (currently used only by integration tests).

func (r *userSubscriptionRepository) ListExpired(ctx context.Context) ([]service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	subs, err := client.UserSubscription.Query().
		Where(
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.ExpiresAtLTE(time.Now()),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return userSubscriptionEntitiesToService(subs), nil
}

func (r *userSubscriptionRepository) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	count, err := client.UserSubscription.Query().Where(usersubscription.GroupIDEQ(groupID)).Count(ctx)
	return int64(count), err
}

func (r *userSubscriptionRepository) CountActiveByGroupID(ctx context.Context, groupID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	count, err := client.UserSubscription.Query().
		Where(
			usersubscription.GroupIDEQ(groupID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			activeSubscriptionExpiresAt(time.Now()),
		).
		Count(ctx)
	return int64(count), err
}

func (r *userSubscriptionRepository) DeleteByGroupID(ctx context.Context, groupID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	n, err := client.UserSubscription.Delete().Where(usersubscription.GroupIDEQ(groupID)).Exec(ctx)
	return int64(n), err
}

func userSubscriptionEntityToService(m *dbent.UserSubscription) *service.UserSubscription {
	if m == nil {
		return nil
	}
	out := &service.UserSubscription{
		ID:                       m.ID,
		UserID:                   m.UserID,
		GroupID:                  m.GroupID,
		StartsAt:                 m.StartsAt,
		ExpiresAt:                normalizeSubscriptionExpiresAt(m.ExpiresAt),
		Status:                   m.Status,
		DailyWindowStart:         m.DailyWindowStart,
		WeeklyWindowStart:        m.WeeklyWindowStart,
		MonthlyWindowStart:       m.MonthlyWindowStart,
		DailyUsageUSD:            m.DailyUsageUsd,
		WeeklyUsageUSD:           m.WeeklyUsageUsd,
		MonthlyUsageUSD:          m.MonthlyUsageUsd,
		ResetCount:               m.ResetCount,
		EarlyResetEnabled:        m.EarlyResetEnabled,
		EarlyResetDurationDays:   m.EarlyResetDurationDays,
		AssignedBy:               m.AssignedBy,
		AssignedAt:               m.AssignedAt,
		Notes:                    derefString(m.Notes),
		PlanConcurrency:          m.PlanConcurrency,
		PlanConcurrencyExpiresAt: m.PlanConcurrencyExpiresAt,
		CustomMultiplier:         m.CustomMultiplier,
		CustomSourcePlanID:       m.CustomSourcePlanID,
		CustomSourceGroupID:      m.CustomSourceGroupID,
		CustomExpiresAt:          m.CustomExpiresAt,
		CustomDisplayName:        derefString(m.CustomDisplayName),
		CreatedAt:                m.CreatedAt,
		UpdatedAt:                m.UpdatedAt,
	}
	if m.Edges.User != nil {
		out.User = userEntityToService(m.Edges.User)
	}
	if m.Edges.Group != nil {
		out.Group = groupEntityToService(m.Edges.Group)
	}
	if m.Edges.AssignedByUser != nil {
		out.AssignedByUser = userEntityToService(m.Edges.AssignedByUser)
	}
	return out
}

func userSubscriptionEntitiesToService(models []*dbent.UserSubscription) []service.UserSubscription {
	out := make([]service.UserSubscription, 0, len(models))
	for i := range models {
		if s := userSubscriptionEntityToService(models[i]); s != nil {
			out = append(out, *s)
		}
	}
	return out
}

func nilIfEmptyString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func applyUserSubscriptionEntityToService(dst *service.UserSubscription, src *dbent.UserSubscription) {
	if dst == nil || src == nil {
		return
	}
	dst.ID = src.ID
	dst.ResetCount = src.ResetCount
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
}
