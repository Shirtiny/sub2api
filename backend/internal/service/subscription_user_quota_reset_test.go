//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type userQuotaResetRepoStub struct {
	userSubRepoNoop
	sub         *UserSubscription
	resetCalled bool
	resetInput  UserQuotaResetParams
	setCount    int
	setCalled   bool
	bulkIDs     []int64
}

func (r *userQuotaResetRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *userQuotaResetRepoStub) ResetUserQuota(_ context.Context, input UserQuotaResetParams) (*UserSubscription, error) {
	r.resetCalled = true
	r.resetInput = input
	if r.sub.ResetCount <= 0 {
		return nil, ErrQuotaResetExhausted
	}
	r.sub.ResetCount--
	r.sub.DailyUsageUSD = 0
	r.sub.WeeklyUsageUSD = 0
	r.sub.DailyWindowStart = &input.WindowStart
	r.sub.WeeklyWindowStart = &input.WindowStart
	cp := *r.sub
	return &cp, nil
}

func (r *userQuotaResetRepoStub) SetResetCount(_ context.Context, _ int64, count int) error {
	r.setCalled = true
	r.setCount = count
	if r.sub == nil {
		return ErrSubscriptionNotFound
	}
	r.sub.ResetCount = count
	return nil
}

func (r *userQuotaResetRepoStub) BulkSetResetCount(_ context.Context, ids []int64, count int) (int64, error) {
	r.bulkIDs = append([]int64(nil), ids...)
	if r.sub != nil {
		r.sub.ResetCount = count
	}
	return int64(len(ids)), nil
}

func activeResetTestSub() *UserSubscription {
	now := time.Now()
	daily := now.Add(-time.Hour)
	weekly := now.Add(-time.Hour)
	return &UserSubscription{
		ID: 1, UserID: 2, GroupID: 3,
		StartsAt: now.Add(-24 * time.Hour), ExpiresAt: now.Add(24 * time.Hour),
		Status:           SubscriptionStatusActive,
		DailyWindowStart: &daily, WeeklyWindowStart: &weekly,
		DailyUsageUSD: 4, WeeklyUsageUSD: 5, MonthlyUsageUSD: 6,
		ResetCount: 2,
	}
}

func TestResetUserQuotaConsumesOneAllowanceAndOnlyDailyWeekly(t *testing.T) {
	repo := &userQuotaResetRepoStub{sub: activeResetTestSub()}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)

	updated, err := svc.ResetUserQuota(context.Background(), 2, 1)

	require.NoError(t, err)
	require.True(t, repo.resetCalled)
	require.Equal(t, 1, updated.ResetCount)
	require.Zero(t, updated.DailyUsageUSD)
	require.Zero(t, updated.WeeklyUsageUSD)
	require.Equal(t, float64(6), updated.MonthlyUsageUSD)
	require.False(t, repo.resetInput.WindowStart.IsZero())
}

func TestResetUserQuotaRejectsWrongOwnerEarlyResetAndExhausted(t *testing.T) {
	repo := &userQuotaResetRepoStub{sub: activeResetTestSub()}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)
	_, err := svc.ResetUserQuota(context.Background(), 99, 1)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	require.False(t, repo.resetCalled)

	repo.sub.EarlyResetEnabled = true
	_, err = svc.ResetUserQuota(context.Background(), 2, 1)
	require.ErrorIs(t, err, ErrQuotaResetDisabled)
	require.False(t, repo.resetCalled)

	repo.sub.EarlyResetEnabled = false
	repo.sub.ResetCount = 0
	_, err = svc.ResetUserQuota(context.Background(), 2, 1)
	require.ErrorIs(t, err, ErrQuotaResetExhausted)
	require.False(t, repo.resetCalled)
}

func TestSetSubscriptionResetCountValidatesBounds(t *testing.T) {
	repo := &userQuotaResetRepoStub{sub: activeResetTestSub()}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)
	_, err := svc.SetSubscriptionResetCount(context.Background(), 1, -1)
	require.ErrorIs(t, err, ErrInvalidQuotaResetCount)
	_, err = svc.SetSubscriptionResetCount(context.Background(), 1, MaxUserQuotaResetCount+1)
	require.ErrorIs(t, err, ErrInvalidQuotaResetCount)

	updated, err := svc.SetSubscriptionResetCount(context.Background(), 1, 7)
	require.NoError(t, err)
	require.Equal(t, 7, updated.ResetCount)
	require.True(t, repo.setCalled)
}

func TestResetUserQuotaRejectsOneDayCard(t *testing.T) {
	now := time.Now()
	sub := activeResetTestSub()
	sub.StartsAt = now.Add(-12 * time.Hour)
	sub.ExpiresAt = now.Add(12 * time.Hour)
	repo := &userQuotaResetRepoStub{sub: sub}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)

	_, err := svc.ResetUserQuota(context.Background(), sub.UserID, sub.ID)

	require.ErrorIs(t, err, ErrQuotaResetDisabled)
	require.False(t, repo.resetCalled)
}

func TestResetUserQuotaRejectsSubscriptionThatHasNotStarted(t *testing.T) {
	sub := activeResetTestSub()
	sub.StartsAt = time.Now().Add(time.Hour)
	repo := &userQuotaResetRepoStub{sub: sub}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)

	_, err := svc.ResetUserQuota(context.Background(), sub.UserID, sub.ID)

	require.ErrorIs(t, err, ErrSubscriptionNotStarted)
	require.False(t, repo.resetCalled)
}
