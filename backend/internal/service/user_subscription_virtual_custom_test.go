package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEffectiveSubscriptionGroupAppliesActiveVirtualCustomMultiplier(t *testing.T) {
	daily, weekly, monthly := 10.0, 20.0, 30.0
	customExpiresAt := time.Now().Add(time.Hour)
	multiplier := 3
	planID := int64(100)
	sourceGroupID := int64(200)
	sub := &UserSubscription{
		Status:              SubscriptionStatusActive,
		ExpiresAt:           time.Now().Add(24 * time.Hour),
		DailyUsageUSD:       29,
		WeeklyUsageUSD:      60,
		MonthlyUsageUSD:     91,
		CustomMultiplier:    &multiplier,
		CustomSourcePlanID:  &planID,
		CustomSourceGroupID: &sourceGroupID,
		CustomExpiresAt:     &customExpiresAt,
		CustomDisplayName:   "[3x]Plan#1",
	}
	group := &Group{ID: sourceGroupID, Name: "Plan", DailyLimitUSD: &daily, WeeklyLimitUSD: &weekly, MonthlyLimitUSD: &monthly}

	effective := EffectiveSubscriptionGroup(sub, group)
	require.NotSame(t, group, effective)
	require.Equal(t, "[3x]Plan#1", effective.Name)
	require.InDelta(t, 30, *effective.DailyLimitUSD, 1e-9)
	require.InDelta(t, 60, *effective.WeeklyLimitUSD, 1e-9)
	require.InDelta(t, 90, *effective.MonthlyLimitUSD, 1e-9)
	require.True(t, sub.CheckDailyLimit(group, 1))
	require.False(t, sub.CheckMonthlyLimit(group, 0))
}

func TestEffectiveSubscriptionGroupIgnoresExpiredVirtualCustomMultiplier(t *testing.T) {
	daily := 10.0
	customExpiresAt := time.Now().Add(-time.Hour)
	multiplier := 3
	planID := int64(100)
	sourceGroupID := int64(200)
	sub := &UserSubscription{
		Status:              SubscriptionStatusActive,
		ExpiresAt:           time.Now().Add(24 * time.Hour),
		CustomMultiplier:    &multiplier,
		CustomSourcePlanID:  &planID,
		CustomSourceGroupID: &sourceGroupID,
		CustomExpiresAt:     &customExpiresAt,
		CustomDisplayName:   "[3x]Plan#1",
	}
	group := &Group{ID: sourceGroupID, Name: "Plan", DailyLimitUSD: &daily}

	require.False(t, sub.IsVirtualCustomSubscription())
	require.Same(t, group, EffectiveSubscriptionGroup(sub, group))
	require.Nil(t, sub.DisplayCustomSourcePlanID())
	require.Equal(t, "Plan", sub.DisplayName(group))
	require.False(t, sub.CheckDailyLimit(group, 11))
}
