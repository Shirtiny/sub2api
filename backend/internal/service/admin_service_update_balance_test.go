//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type balanceUserRepoStub struct {
	*userRepoStub
	updateErr error
	updated   []*User
}

func (s *balanceUserRepoStub) Update(ctx context.Context, user *User) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	if user == nil {
		return nil
	}
	clone := *user
	s.updated = append(s.updated, &clone)
	if s.userRepoStub != nil {
		s.userRepoStub.user = &clone
	}
	return nil
}

type balanceRedeemRepoStub struct {
	*redeemRepoStub
	created []*RedeemCode
}

func (s *balanceRedeemRepoStub) Create(ctx context.Context, code *RedeemCode) error {
	if code == nil {
		return nil
	}
	clone := *code
	s.created = append(s.created, &clone)
	return nil
}

type authCacheInvalidatorStub struct {
	userIDs  []int64
	groupIDs []int64
	keys     []string
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByKey(ctx context.Context, key string) {
	s.keys = append(s.keys, key)
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	s.userIDs = append(s.userIDs, userID)
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64) {
	s.groupIDs = append(s.groupIDs, groupID)
}

func TestAdminService_UpdateUserBalance_InvalidatesAuthCache(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10}}
	repo := &balanceUserRepoStub{userRepoStub: baseRepo}
	redeemRepo := &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{}}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       redeemRepo,
		authCacheInvalidator: invalidator,
	}

	_, err := svc.UpdateUserBalance(context.Background(), UpdateUserBalanceInput{
		UserID:            7,
		Balance:           5,
		Operation:         "add",
		RecordUserHistory: true,
	})
	require.NoError(t, err)
	require.Equal(t, []int64{7}, invalidator.userIDs)
	require.Len(t, redeemRepo.created, 1)
}

func TestAdminService_UpdateUserBalance_NoChangeNoInvalidate(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10}}
	repo := &balanceUserRepoStub{userRepoStub: baseRepo}
	redeemRepo := &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{}}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       redeemRepo,
		authCacheInvalidator: invalidator,
	}

	_, err := svc.UpdateUserBalance(context.Background(), UpdateUserBalanceInput{
		UserID:            7,
		Balance:           10,
		Operation:         "set",
		RecordUserHistory: true,
	})
	require.NoError(t, err)
	require.Empty(t, invalidator.userIDs)
	require.Empty(t, redeemRepo.created)
}

func TestAdminService_UpdateUserBalance_AuditOnlySkipsRedeemCode(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10}}
	repo := &balanceUserRepoStub{userRepoStub: baseRepo}
	redeemRepo := &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{}}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       redeemRepo,
		authCacheInvalidator: invalidator,
	}

	got, err := svc.UpdateUserBalance(context.Background(), UpdateUserBalanceInput{
		UserID:            7,
		Balance:           5,
		Operation:         "add",
		Notes:             "service outage compensation",
		RecordUserHistory: false,
		OperatorID:        99,
	})
	require.NoError(t, err)
	require.Equal(t, 15.0, got.Balance)
	require.Equal(t, []int64{7}, invalidator.userIDs)
	require.Len(t, repo.updated, 1)
	require.Empty(t, redeemRepo.created)
}

func TestAdminService_UpdateUserMembershipPoints_InvalidatesAuthCache(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, TotalRecharged: 10}}
	repo := &balanceUserRepoStub{userRepoStub: baseRepo}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		authCacheInvalidator: invalidator,
	}

	got, err := svc.UpdateUserMembershipPoints(context.Background(), 7, 5, "add", "")
	require.NoError(t, err)
	require.Equal(t, 15.0, got.TotalRecharged)
	require.Equal(t, []int64{7}, invalidator.userIDs)
	require.Len(t, repo.updated, 1)
}

func TestAdminService_UpdateUserMembershipPoints_RejectsNegativeResult(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, TotalRecharged: 3}}
	repo := &balanceUserRepoStub{userRepoStub: baseRepo}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		authCacheInvalidator: invalidator,
	}

	_, err := svc.UpdateUserMembershipPoints(context.Background(), 7, 5, "subtract", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "membership points cannot be negative")
	require.Empty(t, invalidator.userIDs)
	require.Empty(t, repo.updated)
}
