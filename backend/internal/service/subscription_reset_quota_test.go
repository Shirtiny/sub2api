//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"github.com/stretchr/testify/require"
)

// resetQuotaUserSubRepoStub 支持 GetByID、ResetUsageWindows，
// 其余方法继承 userSubRepoNoop（panic）。
type resetQuotaUserSubRepoStub struct {
	userSubRepoNoop

	sub *UserSubscription

	resetDailyCalled   bool
	resetWeeklyCalled  bool
	resetMonthlyCalled bool
	resetDailyErr      error
	resetWeeklyErr     error
	resetMonthlyErr    error

	listSubs               []UserSubscription
	listErr                error
	resetActiveCalled      bool
	resetActiveDaily       bool
	resetActiveWeekly      bool
	resetActiveMonthly     bool
	resetActiveWindowSet   bool
	resetActiveReturnCount int64
	resetActiveErr         error
}

func (r *resetQuotaUserSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *resetQuotaUserSubRepoStub) ResetUsageWindows(_ context.Context, _ int64, resetDaily, resetWeekly, resetMonthly bool, windowStart time.Time) error {
	r.resetDailyCalled = resetDaily
	r.resetWeeklyCalled = resetWeekly
	r.resetMonthlyCalled = resetMonthly
	if resetDaily && r.resetDailyErr != nil {
		return r.resetDailyErr
	}
	if resetWeekly && r.resetWeeklyErr != nil {
		return r.resetWeeklyErr
	}
	if resetMonthly && r.resetMonthlyErr != nil {
		return r.resetMonthlyErr
	}
	if r.sub == nil {
		return nil
	}
	if resetDaily {
		r.sub.DailyUsageUSD = 0
		r.sub.DailyWindowStart = &windowStart
	}
	if resetWeekly {
		r.sub.WeeklyUsageUSD = 0
		r.sub.WeeklyWindowStart = &windowStart
	}
	if resetMonthly {
		r.sub.MonthlyUsageUSD = 0
		r.sub.MonthlyWindowStart = &windowStart
	}
	return nil
}

func (r *resetQuotaUserSubRepoStub) ResetDailyUsage(_ context.Context, _ int64, _ *time.Time, windowStart time.Time) error {
	r.resetDailyCalled = true
	if r.resetDailyErr == nil && r.sub != nil {
		r.sub.DailyUsageUSD = 0
		r.sub.DailyWindowStart = &windowStart
	}
	return r.resetDailyErr
}

func (r *resetQuotaUserSubRepoStub) ResetWeeklyUsage(_ context.Context, _ int64, _ *time.Time, _ time.Time) error {
	r.resetWeeklyCalled = true
	return r.resetWeeklyErr
}

func (r *resetQuotaUserSubRepoStub) ResetMonthlyUsage(_ context.Context, _ int64, _ *time.Time, _ time.Time) error {
	r.resetMonthlyCalled = true
	return r.resetMonthlyErr
}

func (r *resetQuotaUserSubRepoStub) List(_ context.Context, params pagination.PaginationParams, _ *int64, _ *int64, status, _ string, _ string, _ string) ([]UserSubscription, *pagination.PaginationResult, error) {
	if r.listErr != nil {
		return nil, nil, r.listErr
	}
	if status != SubscriptionStatusActive {
		return nil, nil, ErrInvalidInput
	}
	start := params.Offset()
	if start >= len(r.listSubs) {
		return nil, &pagination.PaginationResult{Total: int64(len(r.listSubs)), Page: params.Page, PageSize: params.Limit(), Pages: 1}, nil
	}
	end := start + params.Limit()
	if end > len(r.listSubs) {
		end = len(r.listSubs)
	}
	pages := (len(r.listSubs) + params.Limit() - 1) / params.Limit()
	return append([]UserSubscription(nil), r.listSubs[start:end]...), &pagination.PaginationResult{Total: int64(len(r.listSubs)), Page: params.Page, PageSize: params.Limit(), Pages: pages}, nil
}

func (r *resetQuotaUserSubRepoStub) ResetActiveUsage(_ context.Context, resetDaily, resetWeekly, resetMonthly bool, windowStart time.Time) (int64, error) {
	r.resetActiveCalled = true
	r.resetActiveDaily = resetDaily
	r.resetActiveWeekly = resetWeekly
	r.resetActiveMonthly = resetMonthly
	r.resetActiveWindowSet = !windowStart.IsZero()
	return r.resetActiveReturnCount, r.resetActiveErr
}

func newResetQuotaSvc(stub *resetQuotaUserSubRepoStub) *SubscriptionService {
	return NewSubscriptionService(groupRepoNoop{}, stub, nil, nil, nil)
}

func TestAdminResetQuota_ResetBoth(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 1, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	result, err := svc.AdminResetQuota(context.Background(), 1, true, true, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, stub.resetDailyCalled, "应调用 ResetDailyUsage")
	require.True(t, stub.resetWeeklyCalled, "应调用 ResetWeeklyUsage")
	require.False(t, stub.resetMonthlyCalled, "不应调用 ResetMonthlyUsage")
}

func TestAdminResetQuota_ResetDailyOnly(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 2, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	result, err := svc.AdminResetQuota(context.Background(), 2, true, false, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, stub.resetDailyCalled, "应调用 ResetDailyUsage")
	require.False(t, stub.resetWeeklyCalled, "不应调用 ResetWeeklyUsage")
	require.False(t, stub.resetMonthlyCalled, "不应调用 ResetMonthlyUsage")
}

func TestAdminResetQuota_ResetWeeklyOnly(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 3, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	result, err := svc.AdminResetQuota(context.Background(), 3, false, true, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, stub.resetDailyCalled, "不应调用 ResetDailyUsage")
	require.True(t, stub.resetWeeklyCalled, "应调用 ResetWeeklyUsage")
	require.False(t, stub.resetMonthlyCalled, "不应调用 ResetMonthlyUsage")
}

func TestAdminResetQuota_BothFalseReturnsError(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 7, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 7, false, false, false)

	require.ErrorIs(t, err, ErrInvalidInput)
	require.False(t, stub.resetDailyCalled)
	require.False(t, stub.resetWeeklyCalled)
	require.False(t, stub.resetMonthlyCalled)
}

func TestAdminResetQuota_SubscriptionNotFound(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{sub: nil}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 999, true, true, true)

	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	require.False(t, stub.resetDailyCalled)
	require.False(t, stub.resetWeeklyCalled)
	require.False(t, stub.resetMonthlyCalled)
}

func TestAdminResetQuota_ResetDailyUsageError(t *testing.T) {
	dbErr := errors.New("db error")
	stub := &resetQuotaUserSubRepoStub{
		sub:           &UserSubscription{ID: 4, UserID: 10, GroupID: 20},
		resetDailyErr: dbErr,
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 4, true, true, false)

	require.ErrorIs(t, err, dbErr)
	require.True(t, stub.resetDailyCalled)
	require.True(t, stub.resetWeeklyCalled, "原子重置应在一次调用中提交所选窗口")
}

func TestAdminResetQuota_ResetWeeklyUsageError(t *testing.T) {
	dbErr := errors.New("db error")
	stub := &resetQuotaUserSubRepoStub{
		sub:            &UserSubscription{ID: 5, UserID: 10, GroupID: 20},
		resetWeeklyErr: dbErr,
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 5, false, true, false)

	require.ErrorIs(t, err, dbErr)
	require.True(t, stub.resetWeeklyCalled)
}

func TestAdminResetQuota_ResetMonthlyOnly(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{ID: 8, UserID: 10, GroupID: 20},
	}
	svc := newResetQuotaSvc(stub)

	result, err := svc.AdminResetQuota(context.Background(), 8, false, false, true)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, stub.resetDailyCalled, "不应调用 ResetDailyUsage")
	require.False(t, stub.resetWeeklyCalled, "不应调用 ResetWeeklyUsage")
	require.True(t, stub.resetMonthlyCalled, "应调用 ResetMonthlyUsage")
}

func TestAdminResetQuota_ResetMonthlyUsageError(t *testing.T) {
	dbErr := errors.New("db error")
	stub := &resetQuotaUserSubRepoStub{
		sub:             &UserSubscription{ID: 9, UserID: 10, GroupID: 20},
		resetMonthlyErr: dbErr,
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 9, false, false, true)

	require.ErrorIs(t, err, dbErr)
	require.True(t, stub.resetMonthlyCalled)
}

func TestAdminResetQuota_ReturnsRefreshedSub(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		sub: &UserSubscription{
			ID:            6,
			UserID:        10,
			GroupID:       20,
			DailyUsageUSD: 99.9,
		},
	}

	svc := newResetQuotaSvc(stub)
	result, err := svc.AdminResetQuota(context.Background(), 6, true, false, false)

	require.NoError(t, err)
	// ResetUsageWindows stub 会将 sub.DailyUsageUSD 归零，
	// 服务应返回第二次 GetByID 的刷新值而非初始的 99.9
	require.Equal(t, float64(0), result.DailyUsageUSD, "返回的订阅应反映已归零的用量")
	require.True(t, stub.resetDailyCalled)
}

func TestAdminBulkResetQuota_ResetAllActive(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{
		listSubs: []UserSubscription{
			{ID: 1, UserID: 10, GroupID: 20},
			{ID: 2, UserID: 11, GroupID: 21},
		},
		resetActiveReturnCount: 2,
	}

	svc := newResetQuotaSvc(stub)
	count, err := svc.AdminBulkResetQuota(context.Background(), true, true, true)

	require.NoError(t, err)
	require.Equal(t, int64(2), count)
	require.True(t, stub.resetActiveCalled)
	require.True(t, stub.resetActiveDaily)
	require.True(t, stub.resetActiveWeekly)
	require.True(t, stub.resetActiveMonthly)
	require.True(t, stub.resetActiveWindowSet)
}

func TestAdminBulkResetQuota_AllFalseReturnsError(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{}
	svc := newResetQuotaSvc(stub)

	count, err := svc.AdminBulkResetQuota(context.Background(), false, false, false)

	require.ErrorIs(t, err, ErrInvalidInput)
	require.Equal(t, int64(0), count)
	require.False(t, stub.resetActiveCalled)
}

func TestAdminBulkResetQuota_ListError(t *testing.T) {
	listErr := errors.New("list failed")
	stub := &resetQuotaUserSubRepoStub{listErr: listErr}
	svc := newResetQuotaSvc(stub)

	count, err := svc.AdminBulkResetQuota(context.Background(), true, true, true)

	require.ErrorIs(t, err, listErr)
	require.Equal(t, int64(0), count)
	require.False(t, stub.resetActiveCalled)
}

func TestAdminBulkResetQuota_ResetError(t *testing.T) {
	resetErr := errors.New("reset failed")
	stub := &resetQuotaUserSubRepoStub{
		listSubs:       []UserSubscription{{ID: 1, UserID: 10, GroupID: 20}},
		resetActiveErr: resetErr,
	}
	svc := newResetQuotaSvc(stub)

	count, err := svc.AdminBulkResetQuota(context.Background(), true, true, true)

	require.ErrorIs(t, err, resetErr)
	require.Equal(t, int64(0), count)
	require.True(t, stub.resetActiveCalled)
}

func TestEnsureWindowMaintenance_VirtualCustomSubscriptionPreservesEntitlement(t *testing.T) {
	now := time.Now()
	windowStart := now.Add(-25 * time.Hour)
	customExpiresAt := now.Add(15 * 24 * time.Hour)
	multiplier := 3
	planID := int64(100)
	sourceGroupID := int64(200)
	sourceDailyLimit := 10.0
	sub := &UserSubscription{
		ID:                  11,
		UserID:              21,
		GroupID:             sourceGroupID,
		Status:              SubscriptionStatusActive,
		StartsAt:            now.Add(-24 * time.Hour),
		ExpiresAt:           now.Add(30 * 24 * time.Hour),
		DailyUsageUSD:       31,
		DailyWindowStart:    &windowStart,
		CustomMultiplier:    &multiplier,
		CustomSourcePlanID:  &planID,
		CustomSourceGroupID: &sourceGroupID,
		CustomExpiresAt:     &customExpiresAt,
		CustomDisplayName:   "Custom Plan-3x",
	}
	group := &Group{ID: sourceGroupID, DailyLimitUSD: &sourceDailyLimit}
	stub := &resetQuotaUserSubRepoStub{sub: sub}
	svc := newResetQuotaSvc(stub)

	needsMaintenance, err := svc.ValidateAndCheckLimits(sub, group)
	require.NoError(t, err)
	require.True(t, needsMaintenance)

	refreshed, err := svc.EnsureWindowMaintenance(context.Background(), sub)
	require.NoError(t, err)
	require.True(t, stub.resetDailyCalled)
	require.Zero(t, refreshed.DailyUsageUSD)
	require.Equal(t, multiplier, *refreshed.CustomMultiplier)
	require.Equal(t, planID, *refreshed.CustomSourcePlanID)
	require.Equal(t, sourceGroupID, *refreshed.CustomSourceGroupID)
	require.Equal(t, "Custom Plan-3x", refreshed.CustomDisplayName)
	require.True(t, refreshed.CheckDailyLimit(group, 30))
	require.False(t, refreshed.CheckDailyLimit(group, 30.01))
}

func TestEnsureWindowMaintenance_LegacyCustomGroupPreservesMultipliedLimit(t *testing.T) {
	now := time.Now()
	windowStart := now.Add(-25 * time.Hour)
	customDailyLimit := 20.0
	multiplier := 2
	sub := &UserSubscription{
		ID:               12,
		UserID:           22,
		GroupID:          202,
		Status:           SubscriptionStatusActive,
		StartsAt:         now.Add(-24 * time.Hour),
		ExpiresAt:        now.Add(30 * 24 * time.Hour),
		DailyUsageUSD:    21,
		DailyWindowStart: &windowStart,
	}
	group := &Group{
		ID:                        202,
		IsCustomSubscriptionGroup: true,
		CustomMultiplier:          &multiplier,
		DailyLimitUSD:             &customDailyLimit,
	}
	stub := &resetQuotaUserSubRepoStub{sub: sub}
	svc := newResetQuotaSvc(stub)

	needsMaintenance, err := svc.ValidateAndCheckLimits(sub, group)
	require.NoError(t, err)
	require.True(t, needsMaintenance)

	refreshed, err := svc.EnsureWindowMaintenance(context.Background(), sub)
	require.NoError(t, err)
	require.True(t, stub.resetDailyCalled)
	require.Zero(t, refreshed.DailyUsageUSD)
	require.True(t, group.IsCustomSubscriptionGroup)
	require.Equal(t, multiplier, *group.CustomMultiplier)
	require.InDelta(t, 20, *group.DailyLimitUSD, 1e-9)
	require.True(t, refreshed.CheckDailyLimit(group, 20))
	require.False(t, refreshed.CheckDailyLimit(group, 20.01))
}
