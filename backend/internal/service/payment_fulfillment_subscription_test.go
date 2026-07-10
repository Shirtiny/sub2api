//go:build unit

package service

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type paymentFulfillmentSubscriptionRepo struct {
	client *dbent.Client
}

func (r *paymentFulfillmentSubscriptionRepo) Create(ctx context.Context, sub *UserSubscription) error {
	if sub == nil {
		return ErrSubscriptionNilInput
	}
	client := paymentFulfillmentSubscriptionClientFromContext(ctx, r.client)
	builder := client.UserSubscription.Create().
		SetUserID(sub.UserID).
		SetGroupID(sub.GroupID).
		SetExpiresAt(sub.ExpiresAt).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetNotes(sub.Notes).
		SetNillableCustomMultiplier(sub.CustomMultiplier).
		SetNillableCustomSourcePlanID(sub.CustomSourcePlanID).
		SetNillableCustomSourceGroupID(sub.CustomSourceGroupID).
		SetNillableCustomExpiresAt(sub.CustomExpiresAt).
		SetNillableCustomDisplayName(nillableString(sub.CustomDisplayName))
	if !sub.StartsAt.IsZero() {
		builder.SetStartsAt(sub.StartsAt)
	}
	if sub.Status != "" {
		builder.SetStatus(sub.Status)
	}
	if !sub.AssignedAt.IsZero() {
		builder.SetAssignedAt(sub.AssignedAt)
	}
	if sub.AssignedBy != nil {
		builder.SetAssignedBy(*sub.AssignedBy)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	sub.ID = created.ID
	return nil
}

func (r *paymentFulfillmentSubscriptionRepo) GetByID(ctx context.Context, id int64) (*UserSubscription, error) {
	client := paymentFulfillmentSubscriptionClientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Get(ctx, id)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}
	return &UserSubscription{
		ID:                  m.ID,
		UserID:              m.UserID,
		GroupID:             m.GroupID,
		StartsAt:            m.StartsAt,
		ExpiresAt:           m.ExpiresAt,
		Status:              m.Status,
		AssignedAt:          m.AssignedAt,
		AssignedBy:          m.AssignedBy,
		Notes:               derefString(m.Notes),
		DailyUsageUSD:       m.DailyUsageUsd,
		WeeklyUsageUSD:      m.WeeklyUsageUsd,
		MonthlyUsageUSD:     m.MonthlyUsageUsd,
		DailyWindowStart:    m.DailyWindowStart,
		WeeklyWindowStart:   m.WeeklyWindowStart,
		MonthlyWindowStart:  m.MonthlyWindowStart,
		CustomMultiplier:    m.CustomMultiplier,
		CustomSourcePlanID:  m.CustomSourcePlanID,
		CustomSourceGroupID: m.CustomSourceGroupID,
		CustomExpiresAt:     m.CustomExpiresAt,
		CustomDisplayName:   derefString(m.CustomDisplayName),
	}, nil
}

func (r *paymentFulfillmentSubscriptionRepo) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*UserSubscription, error) {
	client := paymentFulfillmentSubscriptionClientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID), usersubscription.GroupIDEQ(groupID)).
		Only(ctx)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}
	return &UserSubscription{
		ID:                  m.ID,
		UserID:              m.UserID,
		GroupID:             m.GroupID,
		StartsAt:            m.StartsAt,
		ExpiresAt:           m.ExpiresAt,
		Status:              m.Status,
		AssignedAt:          m.AssignedAt,
		AssignedBy:          m.AssignedBy,
		Notes:               derefString(m.Notes),
		DailyUsageUSD:       m.DailyUsageUsd,
		WeeklyUsageUSD:      m.WeeklyUsageUsd,
		MonthlyUsageUSD:     m.MonthlyUsageUsd,
		DailyWindowStart:    m.DailyWindowStart,
		WeeklyWindowStart:   m.WeeklyWindowStart,
		MonthlyWindowStart:  m.MonthlyWindowStart,
		CustomMultiplier:    m.CustomMultiplier,
		CustomSourcePlanID:  m.CustomSourcePlanID,
		CustomSourceGroupID: m.CustomSourceGroupID,
		CustomExpiresAt:     m.CustomExpiresAt,
		CustomDisplayName:   derefString(m.CustomDisplayName),
	}, nil
}

func (r *paymentFulfillmentSubscriptionRepo) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*UserSubscription, error) {
	client := paymentFulfillmentSubscriptionClientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(userID),
			usersubscription.GroupIDEQ(groupID),
			usersubscription.StatusEQ(SubscriptionStatusActive),
			usersubscription.ExpiresAtGT(time.Now()),
		).
		Only(ctx)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}
	return &UserSubscription{
		ID:                  m.ID,
		UserID:              m.UserID,
		GroupID:             m.GroupID,
		StartsAt:            m.StartsAt,
		ExpiresAt:           m.ExpiresAt,
		Status:              m.Status,
		AssignedAt:          m.AssignedAt,
		AssignedBy:          m.AssignedBy,
		Notes:               derefString(m.Notes),
		DailyUsageUSD:       m.DailyUsageUsd,
		WeeklyUsageUSD:      m.WeeklyUsageUsd,
		MonthlyUsageUSD:     m.MonthlyUsageUsd,
		DailyWindowStart:    m.DailyWindowStart,
		WeeklyWindowStart:   m.WeeklyWindowStart,
		MonthlyWindowStart:  m.MonthlyWindowStart,
		CustomMultiplier:    m.CustomMultiplier,
		CustomSourcePlanID:  m.CustomSourcePlanID,
		CustomSourceGroupID: m.CustomSourceGroupID,
		CustomExpiresAt:     m.CustomExpiresAt,
		CustomDisplayName:   derefString(m.CustomDisplayName),
	}, nil
}
func (r *paymentFulfillmentSubscriptionRepo) Update(ctx context.Context, sub *UserSubscription) error {
	if sub == nil {
		return ErrSubscriptionNilInput
	}
	client := paymentFulfillmentSubscriptionClientFromContext(ctx, r.client)
	builder := client.UserSubscription.UpdateOneID(sub.ID).
		SetUserID(sub.UserID).
		SetGroupID(sub.GroupID).
		SetStartsAt(sub.StartsAt).
		SetExpiresAt(sub.ExpiresAt).
		SetStatus(sub.Status).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetNotes(sub.Notes)
	if sub.CustomMultiplier != nil {
		builder.SetCustomMultiplier(*sub.CustomMultiplier)
	} else {
		builder.ClearCustomMultiplier()
	}
	if sub.CustomSourcePlanID != nil {
		builder.SetCustomSourcePlanID(*sub.CustomSourcePlanID)
	} else {
		builder.ClearCustomSourcePlanID()
	}
	if sub.CustomSourceGroupID != nil {
		builder.SetCustomSourceGroupID(*sub.CustomSourceGroupID)
	} else {
		builder.ClearCustomSourceGroupID()
	}
	if sub.CustomExpiresAt != nil {
		builder.SetCustomExpiresAt(*sub.CustomExpiresAt)
	} else {
		builder.ClearCustomExpiresAt()
	}
	if sub.CustomDisplayName != "" {
		builder.SetCustomDisplayName(sub.CustomDisplayName)
	} else {
		builder.ClearCustomDisplayName()
	}
	_, err := builder.Save(ctx)
	return err
}
func (r *paymentFulfillmentSubscriptionRepo) Delete(ctx context.Context, id int64) error {
	client := paymentFulfillmentSubscriptionClientFromContext(ctx, r.client)
	_, err := client.UserSubscription.Delete().Where(usersubscription.IDEQ(id)).Exec(ctx)
	return err
}
func (r *paymentFulfillmentSubscriptionRepo) ListByUserID(context.Context, int64) ([]UserSubscription, error) {
	panic("unexpected ListByUserID call")
}
func (r *paymentFulfillmentSubscriptionRepo) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	panic("unexpected ListActiveByUserID call")
}
func (r *paymentFulfillmentSubscriptionRepo) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID call")
}
func (r *paymentFulfillmentSubscriptionRepo) List(context.Context, pagination.PaginationParams, *int64, *int64, string, string, string, string) ([]UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}
func (r *paymentFulfillmentSubscriptionRepo) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	panic("unexpected ExistsByUserIDAndGroupID call")
}
func (r *paymentFulfillmentSubscriptionRepo) ExtendExpiry(ctx context.Context, id int64, expiresAt time.Time) error {
	client := paymentFulfillmentSubscriptionClientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(id).SetExpiresAt(expiresAt).Save(ctx)
	return err
}
func (r *paymentFulfillmentSubscriptionRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	client := paymentFulfillmentSubscriptionClientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(id).SetStatus(status).Save(ctx)
	return err
}
func (r *paymentFulfillmentSubscriptionRepo) UpdateNotes(ctx context.Context, id int64, notes string) error {
	client := paymentFulfillmentSubscriptionClientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(id).SetNotes(notes).Save(ctx)
	return err
}
func (r *paymentFulfillmentSubscriptionRepo) ActivateWindows(context.Context, int64, time.Time) error {
	panic("unexpected ActivateWindows call")
}
func (r *paymentFulfillmentSubscriptionRepo) ResetUsageWindows(context.Context, int64, bool, bool, bool, time.Time) error {
	panic("unexpected ResetUsageWindows call")
}
func (r *paymentFulfillmentSubscriptionRepo) ResetDailyUsage(context.Context, int64, *time.Time, time.Time) error {
	panic("unexpected ResetDailyUsage call")
}
func (r *paymentFulfillmentSubscriptionRepo) ResetWeeklyUsage(context.Context, int64, *time.Time, time.Time) error {
	panic("unexpected ResetWeeklyUsage call")
}
func (r *paymentFulfillmentSubscriptionRepo) ResetMonthlyUsage(context.Context, int64, *time.Time, time.Time) error {
	panic("unexpected ResetMonthlyUsage call")
}
func (r *paymentFulfillmentSubscriptionRepo) ResetActiveUsage(context.Context, bool, bool, bool, time.Time) (int64, error) {
	panic("unexpected ResetActiveUsage call")
}
func (r *paymentFulfillmentSubscriptionRepo) IncrementUsage(context.Context, int64, float64) error {
	panic("unexpected IncrementUsage call")
}
func (r *paymentFulfillmentSubscriptionRepo) BatchUpdateExpiredStatus(context.Context) (int64, error) {
	panic("unexpected BatchUpdateExpiredStatus call")
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func nillableString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func paymentFulfillmentSubscriptionClientFromContext(ctx context.Context, fallback *dbent.Client) *dbent.Client {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return fallback
}

func TestExecuteSubscriptionFulfillmentAddsMembershipPointsOnce(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().
		SetEmail("subscription-points@example.com").
		SetPasswordHash("hash").
		SetUsername("subscription-points-user").
		Save(ctx)
	require.NoError(t, err)

	group, err := client.Group.Create().
		SetName("subscription-group").
		SetStatus(StatusActive).
		SetPlatform(PlatformAnthropic).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(12.5).
		SetPayAmount(12.5).
		SetFeeRate(0).
		SetRechargeCode("SUBSCRIPTION-POINTS-001").
		SetOutTradeNo("sub2_subscription_points_001").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeSubscription).
		SetSubscriptionGroupID(group.ID).
		SetSubscriptionDays(30).
		SetStatus(OrderStatusPaid).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{
			ID:                  group.ID,
			Name:                group.Name,
			Status:              StatusActive,
			SubscriptionType:    SubscriptionTypeSubscription,
			Hydrated:            true,
			Platform:            PlatformAnthropic,
			RateMultiplier:      1.0,
			DefaultValidityDays: 30,
		},
	}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{
		entClient:       client,
		groupRepo:       groupRepo,
		subscriptionSvc: subSvc,
		providersLoaded: true,
	}

	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))

	updatedUser, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.InDelta(t, 12.5, updatedUser.TotalRecharged, 1e-9)

	updatedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, updatedOrder.Status)

	subCount, err := client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(user.ID), usersubscription.GroupIDEQ(group.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, subCount)

	logCount, err := client.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)),
			paymentauditlog.ActionEQ("SUBSCRIPTION_SUCCESS"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, logCount)

	_, err = client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusFailed).Save(ctx)
	require.NoError(t, err)
	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))

	reloadedUser, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.InDelta(t, 12.5, reloadedUser.TotalRecharged, 1e-9)

	logCount, err = client.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)),
			paymentauditlog.ActionEQ("SUBSCRIPTION_SUCCESS"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, logCount)
}

func TestExecuteSubscriptionFulfillmentRollsBackWhenAuditLogFails(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	user, err := client.User.Create().
		SetEmail("subscription-audit-fail@example.com").
		SetPasswordHash("hash").
		SetUsername("subscription-audit-fail-user").
		Save(ctx)
	require.NoError(t, err)

	group, err := client.Group.Create().
		SetName("subscription-group").
		SetStatus(StatusActive).
		SetPlatform(PlatformAnthropic).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(20).
		SetPayAmount(20).
		SetFeeRate(0).
		SetRechargeCode("SUBSCRIPTION-AUDIT-FAIL-001").
		SetOutTradeNo("sub2_subscription_audit_fail_001").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeSubscription).
		SetSubscriptionGroupID(group.ID).
		SetSubscriptionDays(30).
		SetStatus(OrderStatusPaid).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	var failAuditOnce bool = true
	client.PaymentAuditLog.Use(func(next dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(ctx context.Context, m dbent.Mutation) (dbent.Value, error) {
			if failAuditOnce && m.Op().Is(dbent.OpCreate) {
				if pm, ok := m.(*dbent.PaymentAuditLogMutation); ok {
					if action, exists := pm.Action(); exists && action == "SUBSCRIPTION_SUCCESS" {
						failAuditOnce = false
						return nil, errors.New("forced audit insert failure")
					}
				}
			}
			return next.Mutate(ctx, m)
		})
	})

	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{
			ID:                  group.ID,
			Name:                group.Name,
			Status:              StatusActive,
			SubscriptionType:    SubscriptionTypeSubscription,
			Hydrated:            true,
			Platform:            PlatformAnthropic,
			RateMultiplier:      1.0,
			DefaultValidityDays: 30,
		},
	}
	subRepo := &paymentFulfillmentSubscriptionRepo{client: client}
	subSvc := NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
	svc := &PaymentService{
		entClient:       client,
		groupRepo:       groupRepo,
		subscriptionSvc: subSvc,
		providersLoaded: true,
	}

	err = svc.ExecuteSubscriptionFulfillment(ctx, order.ID)
	require.Error(t, err)

	rolledBackUser, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Zero(t, rolledBackUser.TotalRecharged)

	rolledBackOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusFailed, rolledBackOrder.Status)

	subCount, err := client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(user.ID), usersubscription.GroupIDEQ(group.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, subCount)

	successLogCount, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10))).
		Where(paymentauditlog.ActionEQ("SUBSCRIPTION_SUCCESS")).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, successLogCount)

	failedLogCount, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10))).
		Where(paymentauditlog.ActionEQ("FULFILLMENT_FAILED")).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, failedLogCount)

	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))
	reloadedUser, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.InDelta(t, 20, reloadedUser.TotalRecharged, 1e-9)
}
