package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type earlyResetRepositoryStub struct {
	UserSubscriptionRepository
	sub      *UserSubscription
	input    EarlyResetSubscriptionParams
	called   bool
	err      error
	getCalls int
}

type earlyResetExtensionRepositoryStub struct {
	UserSubscriptionRepository
	sub *UserSubscription
}

func (r *earlyResetExtensionRepositoryStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	copy := *r.sub
	return &copy, nil
}

func (r *earlyResetExtensionRepositoryStub) ExtendExpiry(_ context.Context, id int64, expiresAt time.Time) error {
	if r.sub == nil || r.sub.ID != id {
		return ErrSubscriptionNotFound
	}
	r.sub.ExpiresAt = expiresAt
	return nil
}

func (r *earlyResetRepositoryStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	r.getCalls++
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	copy := *r.sub
	return &copy, nil
}

func TestEarlyResetSubscriptionDoesNotReadAfterMutation(t *testing.T) {
	now := time.Now()
	repo := &earlyResetRepositoryStub{sub: &UserSubscription{
		ID: 10, UserID: 20, GroupID: 30, StartsAt: now.Add(-time.Hour),
		ExpiresAt: now.AddDate(0, 0, 20), Status: SubscriptionStatusActive,
		EarlyResetEnabled: true, EarlyResetDurationDays: 1,
	}}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)

	_, err := svc.EarlyResetSubscription(context.Background(), 20, 10)
	require.NoError(t, err)
	require.Equal(t, 1, repo.getCalls)
}

func (r *earlyResetRepositoryStub) EarlyReset(_ context.Context, input EarlyResetSubscriptionParams) error {
	r.called = true
	r.input = input
	if r.err != nil {
		return r.err
	}
	r.sub.ExpiresAt = input.NewExpiresAt
	r.sub.CustomExpiresAt = cloneTimePtr(input.NewCustomExpiresAt)
	r.sub.DailyUsageUSD = 0
	r.sub.WeeklyUsageUSD = 0
	r.sub.MonthlyUsageUSD = 0
	r.sub.DailyWindowStart = cloneTimePtr(&input.WindowStart)
	r.sub.WeeklyWindowStart = cloneTimePtr(&input.WindowStart)
	r.sub.MonthlyWindowStart = cloneTimePtr(&input.WindowStart)
	return nil
}

func TestEarlyResetSubscriptionResetsUsageAndShortensTerm(t *testing.T) {
	now := time.Now()
	expiresAt := now.AddDate(0, 0, 30)
	repo := &earlyResetRepositoryStub{sub: &UserSubscription{
		ID:                     10,
		UserID:                 20,
		GroupID:                30,
		StartsAt:               now.AddDate(0, 0, -5),
		ExpiresAt:              expiresAt,
		Status:                 SubscriptionStatusActive,
		DailyUsageUSD:          12,
		WeeklyUsageUSD:         24,
		MonthlyUsageUSD:        48,
		EarlyResetEnabled:      true,
		EarlyResetDurationDays: 7,
	}}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)

	got, err := svc.EarlyResetSubscription(context.Background(), 20, 10)
	require.NoError(t, err)
	require.True(t, repo.called)
	require.Equal(t, expiresAt.AddDate(0, 0, -7), repo.input.NewExpiresAt)
	require.Zero(t, got.DailyUsageUSD)
	require.Zero(t, got.WeeklyUsageUSD)
	require.Zero(t, got.MonthlyUsageUSD)
	require.NotNil(t, got.DailyWindowStart)
}

func TestEarlyResetSubscriptionRejectsDisabledAndWrongOwner(t *testing.T) {
	now := time.Now()
	repo := &earlyResetRepositoryStub{sub: &UserSubscription{
		ID:        10,
		UserID:    20,
		GroupID:   30,
		StartsAt:  now.Add(-time.Hour),
		ExpiresAt: now.Add(24 * time.Hour),
		Status:    SubscriptionStatusActive,
	}}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)

	_, err := svc.EarlyResetSubscription(context.Background(), 20, 10)
	require.ErrorIs(t, err, ErrEarlyResetDisabled)
	require.False(t, repo.called)

	repo.sub.EarlyResetEnabled = true
	repo.sub.EarlyResetDurationDays = 1
	_, err = svc.EarlyResetSubscription(context.Background(), 999, 10)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	require.False(t, repo.called)
}

func TestEarlyResetSubscriptionRejectsResetThatWouldExpire(t *testing.T) {
	now := time.Now()
	repo := &earlyResetRepositoryStub{sub: &UserSubscription{
		ID:                     10,
		UserID:                 20,
		GroupID:                30,
		StartsAt:               now.Add(-time.Hour),
		ExpiresAt:              now.Add(48 * time.Hour),
		Status:                 SubscriptionStatusActive,
		EarlyResetEnabled:      true,
		EarlyResetDurationDays: 3,
	}}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)

	_, err := svc.EarlyResetSubscription(context.Background(), 20, 10)
	require.ErrorIs(t, err, ErrEarlyResetWouldExpire)
	require.False(t, repo.called)
}

func TestEarlyResetSubscriptionCanUseRenewedTime(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("early-reset-renewed-time@example.com").
		SetPasswordHash("hash").
		SetUsername("early-reset-renewed-time").
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetName("early-reset-renewed-time-group").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	now := time.Now()
	startsAt := now.AddDate(0, 0, -27)
	currentTermExpiresAt := startsAt.AddDate(0, 0, 28)
	futureTermExpiresAt := currentTermExpiresAt.AddDate(0, 0, 28)
	lastTermExpiresAt := futureTermExpiresAt.AddDate(0, 0, 28)
	subscription, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(startsAt).
		SetExpiresAt(lastTermExpiresAt).
		SetEarlyResetEnabled(true).
		SetEarlyResetDurationDays(31).
		Save(ctx)
	require.NoError(t, err)
	current, err := client.SubscriptionEarlyResetEntitlement.Create().
		SetUserID(user.ID).
		SetSubscriptionID(subscription.ID).
		SetEnabled(true).
		SetDurationDays(31).
		SetStartsAt(startsAt).
		SetExpiresAt(currentTermExpiresAt).
		Save(ctx)
	require.NoError(t, err)
	future, err := client.SubscriptionEarlyResetEntitlement.Create().
		SetUserID(user.ID).
		SetSubscriptionID(subscription.ID).
		SetEnabled(true).
		SetDurationDays(31).
		SetStartsAt(currentTermExpiresAt).
		SetExpiresAt(futureTermExpiresAt).
		Save(ctx)
	require.NoError(t, err)
	last, err := client.SubscriptionEarlyResetEntitlement.Create().
		SetUserID(user.ID).
		SetSubscriptionID(subscription.ID).
		SetEnabled(true).
		SetDurationDays(31).
		SetStartsAt(futureTermExpiresAt).
		SetExpiresAt(lastTermExpiresAt).
		Save(ctx)
	require.NoError(t, err)

	repo := &earlyResetRepositoryStub{sub: &UserSubscription{
		ID: subscription.ID, UserID: user.ID, GroupID: group.ID,
		StartsAt: startsAt, ExpiresAt: lastTermExpiresAt, Status: SubscriptionStatusActive,
		MonthlyUsageUSD: 230, EarlyResetEnabled: true, EarlyResetDurationDays: 31,
	}}
	svc := NewSubscriptionService(nil, repo, nil, client, nil)
	callStartedAt := time.Now()

	got, err := svc.EarlyResetSubscription(ctx, user.ID, subscription.ID)
	require.NoError(t, err)
	callFinishedAt := time.Now()
	require.Equal(t, lastTermExpiresAt.AddDate(0, 0, -31), got.ExpiresAt)
	require.Zero(t, got.MonthlyUsageUSD)

	currentAfter, err := client.SubscriptionEarlyResetEntitlement.Get(ctx, current.ID)
	require.NoError(t, err)
	require.False(t, currentAfter.ExpiresAt.Before(callStartedAt))
	require.False(t, currentAfter.ExpiresAt.After(callFinishedAt))

	futureAfter, err := client.SubscriptionEarlyResetEntitlement.Get(ctx, future.ID)
	require.NoError(t, err)
	require.True(t, currentTermExpiresAt.AddDate(0, 0, -31).Equal(futureAfter.StartsAt))
	require.True(t, futureTermExpiresAt.AddDate(0, 0, -31).Equal(futureAfter.ExpiresAt))
	require.True(t, futureAfter.StartsAt.Before(futureAfter.ExpiresAt))
	require.True(t, futureAfter.ExpiresAt.Before(callStartedAt), "被完全扣掉的续费期保留为已结束的审计区间")

	lastAfter, err := client.SubscriptionEarlyResetEntitlement.Get(ctx, last.ID)
	require.NoError(t, err)
	require.Equal(t, currentAfter.ExpiresAt, lastAfter.StartsAt)
	require.True(t, lastTermExpiresAt.AddDate(0, 0, -31).Equal(lastAfter.ExpiresAt))
	require.True(t, lastAfter.StartsAt.Before(lastAfter.ExpiresAt))
}

func TestExtendSubscriptionKeepsLimitedQuotaPolicyThroughAdjustedExpiry(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("early-reset-adjustment@example.com").
		SetPasswordHash("hash").
		SetUsername("early-reset-adjustment").
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetName("early-reset-adjustment-group").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	now := time.Now()
	originalExpiry := now.AddDate(0, 0, 10)
	subscription, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now.AddDate(0, 0, -10)).
		SetExpiresAt(originalExpiry).
		SetEarlyResetEnabled(true).
		SetEarlyResetDurationDays(31).
		Save(ctx)
	require.NoError(t, err)
	entitlement, err := client.SubscriptionEarlyResetEntitlement.Create().
		SetUserID(user.ID).
		SetSubscriptionID(subscription.ID).
		SetEnabled(true).
		SetDurationDays(31).
		SetStartsAt(subscription.StartsAt).
		SetExpiresAt(originalExpiry).
		Save(ctx)
	require.NoError(t, err)

	repo := &earlyResetExtensionRepositoryStub{sub: &UserSubscription{
		ID: subscription.ID, UserID: user.ID, GroupID: group.ID,
		StartsAt: subscription.StartsAt, ExpiresAt: originalExpiry, Status: SubscriptionStatusActive,
		EarlyResetEnabled: true, EarlyResetDurationDays: 31,
	}}
	svc := NewSubscriptionService(nil, repo, nil, client, nil)

	got, err := svc.ExtendSubscription(ctx, subscription.ID, 20)
	require.NoError(t, err)
	expectedExpiry := originalExpiry.AddDate(0, 0, 20)
	require.Equal(t, expectedExpiry, got.ExpiresAt)

	entitlementAfter, err := client.SubscriptionEarlyResetEntitlement.Get(ctx, entitlement.ID)
	require.NoError(t, err)
	require.True(t, expectedExpiry.Equal(entitlementAfter.ExpiresAt))
}

func TestEarlyResetSubscriptionShortensActiveVirtualCustomTermOnly(t *testing.T) {
	now := time.Now()
	multiplier := 3
	sourcePlanID := int64(40)
	sourceGroupID := int64(50)
	customExpiresAt := now.AddDate(0, 0, 20)
	baseExpiresAt := now.AddDate(0, 0, 40)
	repo := &earlyResetRepositoryStub{sub: &UserSubscription{
		ID:                     10,
		UserID:                 20,
		GroupID:                30,
		StartsAt:               now.AddDate(0, 0, -5),
		ExpiresAt:              baseExpiresAt,
		Status:                 SubscriptionStatusActive,
		EarlyResetEnabled:      true,
		EarlyResetDurationDays: 5,
		CustomMultiplier:       &multiplier,
		CustomSourcePlanID:     &sourcePlanID,
		CustomSourceGroupID:    &sourceGroupID,
		CustomExpiresAt:        &customExpiresAt,
	}}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)

	got, err := svc.EarlyResetSubscription(context.Background(), 20, 10)
	require.NoError(t, err)
	require.Equal(t, baseExpiresAt, got.ExpiresAt)
	require.NotNil(t, got.CustomExpiresAt)
	require.Equal(t, customExpiresAt.AddDate(0, 0, -5), *got.CustomExpiresAt)
}

func TestEarlyResetSubscriptionReturnsConcurrentUpdateConflict(t *testing.T) {
	now := time.Now()
	repo := &earlyResetRepositoryStub{
		sub: &UserSubscription{
			ID:                     10,
			UserID:                 20,
			GroupID:                30,
			StartsAt:               now.Add(-time.Hour),
			ExpiresAt:              now.AddDate(0, 0, 20),
			Status:                 SubscriptionStatusActive,
			EarlyResetEnabled:      true,
			EarlyResetDurationDays: 1,
		},
		err: ErrEarlyResetConflict,
	}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)

	_, err := svc.EarlyResetSubscription(context.Background(), 20, 10)
	require.ErrorIs(t, err, ErrEarlyResetConflict)
}

func TestValidateAssignEarlyResetConfigAllowsDisabledLegacySnapshot(t *testing.T) {
	enabled := false
	durationDays := 0
	err := validateAssignCustomEntitlement(&AssignSubscriptionInput{
		EarlyResetEnabled:      &enabled,
		EarlyResetDurationDays: &durationDays,
	})
	require.NoError(t, err)
}

func TestCurrentEarlyResetTermUsesPurchasedTermInsteadOfLatestSnapshot(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("early-reset-terms@example.com").
		SetPasswordHash("hash").
		SetUsername("early-reset-terms").
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetName("early-reset-terms-group").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)
	now := time.Now()
	subscription, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(48 * time.Hour)).
		SetEarlyResetEnabled(true).
		SetEarlyResetDurationDays(5).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.SubscriptionEarlyResetEntitlement.Create().
		SetUserID(user.ID).
		SetSubscriptionID(subscription.ID).
		SetEnabled(false).
		SetDurationDays(0).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(time.Hour)).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.SubscriptionEarlyResetEntitlement.Create().
		SetUserID(user.ID).
		SetSubscriptionID(subscription.ID).
		SetEnabled(true).
		SetDurationDays(5).
		SetStartsAt(now.Add(time.Hour)).
		SetExpiresAt(now.Add(48 * time.Hour)).
		Save(ctx)
	require.NoError(t, err)

	svc := &SubscriptionService{entClient: client}
	current, found, err := svc.currentEarlyResetTerm(ctx, &UserSubscription{
		ID: subscription.ID, StartsAt: subscription.StartsAt, ExpiresAt: subscription.ExpiresAt,
		Status: SubscriptionStatusActive, EarlyResetEnabled: true, EarlyResetDurationDays: 5,
	}, now)
	require.NoError(t, err)
	require.True(t, found)
	require.False(t, current.Enabled)
	activeSnapshot := &UserSubscription{
		ID: subscription.ID, StartsAt: subscription.StartsAt, ExpiresAt: subscription.ExpiresAt,
		Status: SubscriptionStatusActive, EarlyResetEnabled: true, EarlyResetDurationDays: 5,
	}
	policyExpiresAt, err := svc.applyCurrentEarlyResetPolicy(ctx, activeSnapshot, now)
	require.NoError(t, err)
	require.False(t, activeSnapshot.EarlyResetEnabled, "下一期的限时额度策略不能提前影响当前普通套餐")
	require.Zero(t, activeSnapshot.EarlyResetDurationDays)
	require.NotNil(t, policyExpiresAt)
	require.True(t, now.Add(time.Hour).Equal(*policyExpiresAt))

	next, found, err := svc.currentEarlyResetTerm(ctx, &UserSubscription{
		ID: subscription.ID, StartsAt: subscription.StartsAt, ExpiresAt: subscription.ExpiresAt,
		Status: SubscriptionStatusActive,
	}, now.Add(2*time.Hour))
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, next.Enabled)
	require.Equal(t, 5, next.DurationDays)
	futureSnapshot := &UserSubscription{
		ID: subscription.ID, StartsAt: subscription.StartsAt, ExpiresAt: subscription.ExpiresAt,
		Status: SubscriptionStatusActive,
	}
	_, err = svc.applyCurrentEarlyResetPolicy(ctx, futureSnapshot, now.Add(2*time.Hour))
	require.NoError(t, err)
	require.True(t, futureSnapshot.EarlyResetEnabled, "当前限时额度套餐不能被订阅主记录中的旧策略降级")
	require.Equal(t, 5, futureSnapshot.EarlyResetDurationDays)
}

func TestCurrentEarlyResetTermKeepsPurchasedPolicyAfterPlanEdit(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("early-reset-current-plan@example.com").
		SetPasswordHash("hash").
		SetUsername("early-reset-current-plan").
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetName("early-reset-current-plan-group").
		SetStatus(StatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("current-policy-plan").
		SetDescription("").
		SetPrice(128).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetForSale(true).
		SetEarlyResetEnabled(true).
		SetEarlyResetDurationDays(2).
		Save(ctx)
	require.NoError(t, err)
	now := time.Now()
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(128).
		SetPayAmount(128).
		SetFeeRate(0).
		SetRechargeCode("EARLY-RESET-CURRENT-PLAN").
		SetOutTradeNo("early_reset_current_plan").
		SetPaymentType("balance").
		SetPaymentTradeNo("").
		SetOrderType("subscription").
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(now.Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("app.example.com").
		SetPlanID(plan.ID).
		SetSubscriptionGroupID(group.ID).
		SetSubscriptionDays(30).
		Save(ctx)
	require.NoError(t, err)
	subscription, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.AddDate(0, 0, 30)).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.SubscriptionEarlyResetEntitlement.Create().
		SetUserID(user.ID).
		SetSubscriptionID(subscription.ID).
		SetSourceOrderID(order.ID).
		SetEnabled(true).
		SetDurationDays(2).
		SetStartsAt(subscription.StartsAt).
		SetExpiresAt(subscription.ExpiresAt).
		Save(ctx)
	require.NoError(t, err)

	svc := &SubscriptionService{entClient: client}
	term, found, err := svc.currentEarlyResetTerm(ctx, &UserSubscription{
		ID: subscription.ID, StartsAt: subscription.StartsAt, ExpiresAt: subscription.ExpiresAt,
		Status: SubscriptionStatusActive,
	}, now)
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, term.Enabled)
	require.Equal(t, 2, term.DurationDays)

	_, err = client.SubscriptionPlan.UpdateOneID(plan.ID).
		SetEarlyResetEnabled(false).
		SetEarlyResetDurationDays(1).
		Save(ctx)
	require.NoError(t, err)
	term, found, err = svc.currentEarlyResetTerm(ctx, &UserSubscription{
		ID: subscription.ID, StartsAt: subscription.StartsAt, ExpiresAt: subscription.ExpiresAt,
		Status: SubscriptionStatusActive,
	}, now)
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, term.Enabled)
	require.Equal(t, 2, term.DurationDays)
}
