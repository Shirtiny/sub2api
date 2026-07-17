package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type accountGroupFenceMutatorRecorder struct {
	addedGroupID    int64
	addedAccountIDs []int64
	removedGroupIDs []int64
	replacedGroupID int64
	replacedIDs     []int64
}

func (r *accountGroupFenceMutatorRecorder) BulkAddAccountsToGroup(_ context.Context, groupID int64, accountIDs []int64) error {
	r.addedGroupID = groupID
	r.addedAccountIDs = append([]int64(nil), accountIDs...)
	return nil
}

func (r *accountGroupFenceMutatorRecorder) RemoveAllAccountsFromGroups(_ context.Context, groupIDs []int64) (int64, error) {
	r.removedGroupIDs = append([]int64(nil), groupIDs...)
	return 3, nil
}

func (r *accountGroupFenceMutatorRecorder) ReplaceAccountsForGroup(_ context.Context, groupID int64, accountIDs []int64) error {
	r.replacedGroupID = groupID
	r.replacedIDs = append([]int64(nil), accountIDs...)
	return nil
}

func TestGroupRepositoryAccountGroupWritesDelegateToSchedulerFence(t *testing.T) {
	recorder := &accountGroupFenceMutatorRecorder{}
	repo := &groupRepository{accountGroupMutator: recorder}
	ctx := context.Background()

	affected, err := repo.DeleteAccountGroupsByGroupID(ctx, 41)
	require.NoError(t, err)
	require.Equal(t, int64(3), affected)
	require.Equal(t, []int64{41}, recorder.removedGroupIDs)

	require.NoError(t, repo.BindAccountsToGroup(ctx, 42, []int64{7, 8}))
	require.Equal(t, int64(42), recorder.addedGroupID)
	require.Equal(t, []int64{7, 8}, recorder.addedAccountIDs)

	require.NoError(t, repo.ReplaceAccountsForGroup(ctx, 43, []int64{9, 10}))
	require.Equal(t, int64(43), recorder.replacedGroupID)
	require.Equal(t, []int64{9, 10}, recorder.replacedIDs)
}
