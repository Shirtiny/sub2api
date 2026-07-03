package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type groupRepoNoop struct{}

func (groupRepoNoop) Create(context.Context, *Group) error { panic("unexpected Create call") }
func (groupRepoNoop) GetByID(context.Context, int64) (*Group, error) {
	panic("unexpected GetByID call")
}
func (groupRepoNoop) GetByIDLite(context.Context, int64) (*Group, error) {
	panic("unexpected GetByIDLite call")
}
func (groupRepoNoop) Update(context.Context, *Group) error { panic("unexpected Update call") }
func (groupRepoNoop) Delete(context.Context, int64) error  { panic("unexpected Delete call") }
func (groupRepoNoop) DeleteCascade(context.Context, int64) ([]int64, error) {
	panic("unexpected DeleteCascade call")
}
func (groupRepoNoop) List(context.Context, pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}
func (groupRepoNoop) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}
func (groupRepoNoop) ListActive(context.Context) ([]Group, error) {
	panic("unexpected ListActive call")
}
func (groupRepoNoop) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform call")
}
func (groupRepoNoop) ExistsByName(context.Context, string) (bool, error) {
	panic("unexpected ExistsByName call")
}
func (groupRepoNoop) GetAccountCount(context.Context, int64) (int64, int64, error) {
	panic("unexpected GetAccountCount call")
}
func (groupRepoNoop) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
}
func (groupRepoNoop) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	panic("unexpected GetAccountIDsByGroupIDs call")
}
func (groupRepoNoop) BindAccountsToGroup(context.Context, int64, []int64) error {
	panic("unexpected BindAccountsToGroup call")
}
func (groupRepoNoop) NotifyGroupChanged(context.Context, int64) error {
	return nil
}

func (groupRepoNoop) UpdateSortOrders(context.Context, []GroupSortOrderUpdate) error {
	panic("unexpected UpdateSortOrders call")
}

type subscriptionGroupRepoStub struct {
	groupRepoNoop
	group *Group
}

func (s *subscriptionGroupRepoStub) GetByID(context.Context, int64) (*Group, error) {
	return s.group, nil
}

type userSubRepoNoop struct{}

func (userSubRepoNoop) Create(context.Context, *UserSubscription) error {
	panic("unexpected Create call")
}
func (userSubRepoNoop) GetByID(context.Context, int64) (*UserSubscription, error) {
	panic("unexpected GetByID call")
}
func (userSubRepoNoop) GetByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	panic("unexpected GetByUserIDAndGroupID call")
}
func (userSubRepoNoop) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	panic("unexpected GetActiveByUserIDAndGroupID call")
}
func (userSubRepoNoop) Update(context.Context, *UserSubscription) error {
	panic("unexpected Update call")
}
func (userSubRepoNoop) Delete(context.Context, int64) error { panic("unexpected Delete call") }
func (userSubRepoNoop) ListByUserID(context.Context, int64) ([]UserSubscription, error) {
	panic("unexpected ListByUserID call")
}
func (userSubRepoNoop) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	panic("unexpected ListActiveByUserID call")
}
func (userSubRepoNoop) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID call")
}
func (userSubRepoNoop) List(context.Context, pagination.PaginationParams, *int64, *int64, string, string, string, string) ([]UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}
func (userSubRepoNoop) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	panic("unexpected ExistsByUserIDAndGroupID call")
}
func (userSubRepoNoop) ExtendExpiry(context.Context, int64, time.Time) error {
	panic("unexpected ExtendExpiry call")
}
func (userSubRepoNoop) UpdateStatus(context.Context, int64, string) error {
	panic("unexpected UpdateStatus call")
}
func (userSubRepoNoop) UpdateNotes(context.Context, int64, string) error {
	panic("unexpected UpdateNotes call")
}
func (userSubRepoNoop) ActivateWindows(context.Context, int64, time.Time) error {
	panic("unexpected ActivateWindows call")
}
func (userSubRepoNoop) ResetDailyUsage(context.Context, int64, time.Time) error {
	panic("unexpected ResetDailyUsage call")
}
func (userSubRepoNoop) ResetWeeklyUsage(context.Context, int64, time.Time) error {
	panic("unexpected ResetWeeklyUsage call")
}
func (userSubRepoNoop) ResetMonthlyUsage(context.Context, int64, time.Time) error {
	panic("unexpected ResetMonthlyUsage call")
}
func (userSubRepoNoop) ResetActiveUsage(context.Context, bool, bool, bool, time.Time) (int64, error) {
	panic("unexpected ResetActiveUsage call")
}
func (userSubRepoNoop) IncrementUsage(context.Context, int64, float64) error {
	panic("unexpected IncrementUsage call")
}
func (userSubRepoNoop) BatchUpdateExpiredStatus(context.Context) (int64, error) {
	panic("unexpected BatchUpdateExpiredStatus call")
}

type subscriptionUserSubRepoStub struct {
	userSubRepoNoop

	nextID      int64
	byID        map[int64]*UserSubscription
	byUserGroup map[string]*UserSubscription
	createCalls int
}

func newSubscriptionUserSubRepoStub() *subscriptionUserSubRepoStub {
	return &subscriptionUserSubRepoStub{
		nextID:      1,
		byID:        make(map[int64]*UserSubscription),
		byUserGroup: make(map[string]*UserSubscription),
	}
}

func (s *subscriptionUserSubRepoStub) key(userID, groupID int64) string {
	return strconvFormatInt(userID) + ":" + strconvFormatInt(groupID)
}

func (s *subscriptionUserSubRepoStub) seed(sub *UserSubscription) {
	if sub == nil {
		return
	}
	cp := *sub
	if cp.ID == 0 {
		cp.ID = s.nextID
		s.nextID++
	}
	s.byID[cp.ID] = &cp
	s.byUserGroup[s.key(cp.UserID, cp.GroupID)] = &cp
}

func (s *subscriptionUserSubRepoStub) ExistsByUserIDAndGroupID(_ context.Context, userID, groupID int64) (bool, error) {
	_, ok := s.byUserGroup[s.key(userID, groupID)]
	return ok, nil
}

func (s *subscriptionUserSubRepoStub) GetByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	sub := s.byUserGroup[s.key(userID, groupID)]
	if sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (s *subscriptionUserSubRepoStub) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	sub := s.byUserGroup[s.key(userID, groupID)]
	if sub == nil || sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(time.Now()) {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (s *subscriptionUserSubRepoStub) Create(_ context.Context, sub *UserSubscription) error {
	if sub == nil {
		return nil
	}
	s.createCalls++
	cp := *sub
	if cp.ID == 0 {
		cp.ID = s.nextID
		s.nextID++
	}
	sub.ID = cp.ID
	s.byID[cp.ID] = &cp
	s.byUserGroup[s.key(cp.UserID, cp.GroupID)] = &cp
	return nil
}

func (s *subscriptionUserSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	sub := s.byID[id]
	if sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (s *subscriptionUserSubRepoStub) Update(_ context.Context, sub *UserSubscription) error {
	if sub == nil {
		return ErrSubscriptionNilInput
	}
	existing := s.byID[sub.ID]
	if existing == nil {
		return ErrSubscriptionNotFound
	}
	oldKey := s.key(existing.UserID, existing.GroupID)
	cp := *sub
	s.byID[cp.ID] = &cp
	if oldKey != s.key(cp.UserID, cp.GroupID) {
		delete(s.byUserGroup, oldKey)
	}
	s.byUserGroup[s.key(cp.UserID, cp.GroupID)] = &cp
	return nil
}

func (s *subscriptionUserSubRepoStub) Delete(_ context.Context, id int64) error {
	existing := s.byID[id]
	if existing == nil {
		return ErrSubscriptionNotFound
	}
	delete(s.byID, id)
	delete(s.byUserGroup, s.key(existing.UserID, existing.GroupID))
	return nil
}

func TestRevokeSubscriptionInvalidatesL1CacheSynchronously(t *testing.T) {
	subRepo := newSubscriptionUserSubRepoStub()
	subRepo.seed(&UserSubscription{
		ID:        88,
		UserID:    7,
		GroupID:   9,
		Status:    SubscriptionStatusActive,
		StartsAt:  time.Now().Add(-time.Hour),
		ExpiresAt: time.Now().Add(time.Hour),
	})
	cfg := &config.Config{}
	cfg.SubscriptionCache.L1Size = 64
	cfg.SubscriptionCache.L1TTLSeconds = 60
	svc := NewSubscriptionService(groupRepoNoop{}, subRepo, nil, nil, cfg)
	t.Cleanup(svc.Stop)

	_, err := svc.GetActiveSubscription(context.Background(), 7, 9)
	require.NoError(t, err)
	require.NoError(t, svc.RevokeSubscription(context.Background(), 88))

	_, err = svc.GetActiveSubscription(context.Background(), 7, 9)
	require.Error(t, err)
	require.Equal(t, "SUBSCRIPTION_NOT_FOUND", infraerrors.Reason(err))
}

func TestAssignSubscriptionReuseWhenSemanticsMatch(t *testing.T) {
	start := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	subRepo.seed(&UserSubscription{
		ID:        10,
		UserID:    1001,
		GroupID:   1,
		StartsAt:  start,
		ExpiresAt: start.AddDate(0, 0, 30),
		Notes:     "init",
	})

	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)
	sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       1001,
		GroupID:      1,
		ValidityDays: 30,
		Notes:        "init",
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), sub.ID)
	require.Equal(t, 0, subRepo.createCalls, "reuse should not create new subscription")
}

func TestAssignSubscriptionConflictWhenSemanticsMismatch(t *testing.T) {
	start := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	subRepo.seed(&UserSubscription{
		ID:        11,
		UserID:    2001,
		GroupID:   1,
		StartsAt:  start,
		ExpiresAt: start.AddDate(0, 0, 30),
		Notes:     "old-note",
	})

	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)
	_, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       2001,
		GroupID:      1,
		ValidityDays: 30,
		Notes:        "new-note",
	})
	require.Error(t, err)
	require.Equal(t, "SUBSCRIPTION_ASSIGN_CONFLICT", infraerrorsReason(err))
	require.Equal(t, 0, subRepo.createCalls, "conflict should not create or mutate existing subscription")
}

func TestBulkAssignSubscriptionCreatedReusedAndConflict(t *testing.T) {
	start := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	// user 1: 语义一致，可 reused
	subRepo.seed(&UserSubscription{
		ID:        21,
		UserID:    1,
		GroupID:   1,
		StartsAt:  start,
		ExpiresAt: start.AddDate(0, 0, 30),
		Notes:     "same-note",
	})
	// user 3: 语义冲突（有效期不一致），应 failed
	subRepo.seed(&UserSubscription{
		ID:        23,
		UserID:    3,
		GroupID:   1,
		StartsAt:  start,
		ExpiresAt: start.AddDate(0, 0, 60),
		Notes:     "same-note",
	})

	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)
	result, err := svc.BulkAssignSubscription(context.Background(), &BulkAssignSubscriptionInput{
		UserIDs:      []int64{1, 2, 3},
		GroupID:      1,
		ValidityDays: 30,
		AssignedBy:   9,
		Notes:        "same-note",
	})
	require.NoError(t, err)
	require.Equal(t, 2, result.SuccessCount)
	require.Equal(t, 1, result.CreatedCount)
	require.Equal(t, 1, result.ReusedCount)
	require.Equal(t, 1, result.FailedCount)
	require.Equal(t, "reused", result.Statuses[1])
	require.Equal(t, "created", result.Statuses[2])
	require.Equal(t, "failed", result.Statuses[3])
	require.Equal(t, 1, subRepo.createCalls)
}

func TestAssignSubscriptionKeepsWorkingWhenIdempotencyStoreUnavailable(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	SetDefaultIdempotencyCoordinator(NewIdempotencyCoordinator(failingIdempotencyRepo{}, DefaultIdempotencyConfig()))
	t.Cleanup(func() {
		SetDefaultIdempotencyCoordinator(nil)
	})

	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)
	sub, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       9001,
		GroupID:      1,
		ValidityDays: 30,
		Notes:        "new",
	})
	require.NoError(t, err)
	require.NotNil(t, sub)
	require.Equal(t, 1, subRepo.createCalls, "semantic idempotent endpoint should not depend on idempotency store availability")
}

func TestNormalizeAssignValidityDays(t *testing.T) {
	require.Equal(t, 30, normalizeAssignValidityDays(0))
	require.Equal(t, 30, normalizeAssignValidityDays(-5))
	require.Equal(t, MaxValidityDays, normalizeAssignValidityDays(MaxValidityDays+100))
	require.Equal(t, 7, normalizeAssignValidityDays(7))
}

func TestDetectAssignSemanticConflictCases(t *testing.T) {
	start := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	base := &UserSubscription{
		UserID:    1,
		GroupID:   1,
		StartsAt:  start,
		ExpiresAt: start.AddDate(0, 0, 30),
		Notes:     "same",
	}

	reason, conflict := detectAssignSemanticConflict(base, &AssignSubscriptionInput{
		UserID:       1,
		GroupID:      1,
		ValidityDays: 30,
		Notes:        "same",
	})
	require.False(t, conflict)
	require.Equal(t, "", reason)

	reason, conflict = detectAssignSemanticConflict(base, &AssignSubscriptionInput{
		UserID:       1,
		GroupID:      1,
		ValidityDays: 60,
		Notes:        "same",
	})
	require.True(t, conflict)
	require.Equal(t, "validity_days_mismatch", reason)

	reason, conflict = detectAssignSemanticConflict(base, &AssignSubscriptionInput{
		UserID:       1,
		GroupID:      1,
		ValidityDays: 30,
		Notes:        "other",
	})
	require.True(t, conflict)
	require.Equal(t, "notes_mismatch", reason)
}

func TestAssignSubscriptionGroupTypeValidation(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeStandard},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)

	_, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       1,
		GroupID:      1,
		ValidityDays: 30,
	})
	require.Error(t, err)
	require.Equal(t, infraerrors.Code(ErrGroupNotSubscriptionType), infraerrors.Code(err))
}

func TestAssignOrExtendCustomFromActiveNormalKeepsBaseExpiryAndSetsCustomExpiry(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := newSubscriptionUserSubRepoStub()
	now := time.Now()
	normalExpiresAt := now.AddDate(0, 0, 365)
	subRepo.seed(&UserSubscription{
		ID:        31,
		UserID:    3001,
		GroupID:   1,
		StartsAt:  now.AddDate(0, 0, -1),
		ExpiresAt: normalExpiresAt,
		Status:    SubscriptionStatusActive,
		Notes:     "normal",
	})
	multiplier := 3
	planID := int64(10)
	sourceGroupID := int64(1)
	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)

	sub, reused, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:              3001,
		GroupID:             1,
		ValidityDays:        30,
		Notes:               "custom order",
		CustomMultiplier:    &multiplier,
		CustomSourcePlanID:  &planID,
		CustomSourceGroupID: &sourceGroupID,
		CustomDisplayName:   "[3x]Plan#3001",
	})
	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, normalExpiresAt, sub.ExpiresAt, "custom purchase must not upgrade a longer active normal term for free")
	require.NotNil(t, sub.CustomExpiresAt)
	require.True(t, sub.CustomExpiresAt.After(now.AddDate(0, 0, 29)))
	require.True(t, sub.CustomExpiresAt.Before(now.AddDate(0, 0, 31)))
	require.True(t, sub.CustomExpiresAt.Before(sub.ExpiresAt))
	require.Equal(t, 3, *sub.CustomMultiplier)
}

func TestAssignOrExtendActiveVirtualCustomExtendsCustomExpiry(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := newSubscriptionUserSubRepoStub()
	now := time.Now()
	customExpiresAt := now.AddDate(0, 0, 10)
	multiplier := 2
	planID := int64(20)
	sourceGroupID := int64(1)
	subRepo.seed(&UserSubscription{
		ID:                  32,
		UserID:              3002,
		GroupID:             1,
		StartsAt:            now.AddDate(0, 0, -20),
		ExpiresAt:           customExpiresAt,
		Status:              SubscriptionStatusActive,
		Notes:               "custom",
		CustomMultiplier:    &multiplier,
		CustomSourcePlanID:  &planID,
		CustomSourceGroupID: &sourceGroupID,
		CustomExpiresAt:     &customExpiresAt,
		CustomDisplayName:   "[2x]Plan#3002",
	})
	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)

	sub, reused, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:              3002,
		GroupID:             1,
		ValidityDays:        30,
		Notes:               "renew",
		CustomMultiplier:    &multiplier,
		CustomSourcePlanID:  &planID,
		CustomSourceGroupID: &sourceGroupID,
		CustomDisplayName:   "[2x]Plan#3002",
	})
	require.NoError(t, err)
	require.True(t, reused)
	require.NotNil(t, sub.CustomExpiresAt)
	require.Equal(t, customExpiresAt.AddDate(0, 0, 30), *sub.CustomExpiresAt)
	require.Equal(t, *sub.CustomExpiresAt, sub.ExpiresAt)
}

func TestAssignOrExtendActiveVirtualCustomRejectsPlainRenewal(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := newSubscriptionUserSubRepoStub()
	now := time.Now()
	customExpiresAt := now.AddDate(0, 0, 10)
	multiplier := 2
	planID := int64(30)
	sourceGroupID := int64(1)
	subRepo.seed(&UserSubscription{
		ID:                  33,
		UserID:              3003,
		GroupID:             1,
		StartsAt:            now.AddDate(0, 0, -20),
		ExpiresAt:           customExpiresAt,
		Status:              SubscriptionStatusActive,
		Notes:               "custom",
		CustomMultiplier:    &multiplier,
		CustomSourcePlanID:  &planID,
		CustomSourceGroupID: &sourceGroupID,
		CustomExpiresAt:     &customExpiresAt,
	})
	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)

	_, _, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{UserID: 3003, GroupID: 1, ValidityDays: 30, Notes: "plain"})
	require.Error(t, err)
	require.Equal(t, "SUBSCRIPTION_ASSIGN_CONFLICT", infraerrorsReason(err))
}

func TestExtendSubscriptionDeductsVirtualCustomOverlayBeforeBaseTerm(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription}}
	subRepo := newSubscriptionUserSubRepoStub()
	now := time.Now()
	baseExpiresAt := now.AddDate(0, 0, 365)
	customExpiresAt := now.AddDate(0, 0, 30)
	multiplier := 4
	planID := int64(40)
	sourceGroupID := int64(1)
	subRepo.seed(&UserSubscription{
		ID:                  34,
		UserID:              3004,
		GroupID:             1,
		StartsAt:            now.AddDate(0, 0, -1),
		ExpiresAt:           baseExpiresAt,
		Status:              SubscriptionStatusActive,
		CustomMultiplier:    &multiplier,
		CustomSourcePlanID:  &planID,
		CustomSourceGroupID: &sourceGroupID,
		CustomExpiresAt:     &customExpiresAt,
	})
	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)

	sub, err := svc.ExtendSubscription(context.Background(), 34, -30)
	require.NoError(t, err)
	require.Equal(t, baseExpiresAt, sub.ExpiresAt)
	require.NotNil(t, sub.CustomExpiresAt)
	require.False(t, sub.HasActiveVirtualCustomEntitlementAt(time.Now()))

	restored, err := svc.ExtendSubscription(context.Background(), 34, 30)
	require.NoError(t, err)
	require.Equal(t, baseExpiresAt, restored.ExpiresAt)
	require.NotNil(t, restored.CustomExpiresAt)
	require.True(t, restored.CustomExpiresAt.After(time.Now().AddDate(0, 0, 29)))
	require.True(t, restored.HasActiveVirtualCustomEntitlementAt(time.Now()))
}

func strconvFormatInt(v int64) string {
	return strconv.FormatInt(v, 10)
}

func infraerrorsReason(err error) string {
	return infraerrors.Reason(err)
}
