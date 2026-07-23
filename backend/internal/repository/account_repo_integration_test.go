//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/accountgroup"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type AccountRepoSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client
	repo   *accountRepository
}

type schedulerCacheRecorder struct {
	setAccounts []*service.Account
	deleteIDs   []int64
	accounts    map[int64]*service.Account
}

type schedulerMutationCacheRecorder struct {
	schedulerCacheRecorder
	nextEpoch       int64
	beginErr        error
	publishErr      error
	onBegin         func()
	beginCalls      [][]int64
	publishCalls    map[int64]int64
	publishAttempts map[int64]int
	completeCalls   map[int64]int64
}

func (s *schedulerMutationCacheRecorder) BeginAccountMutations(_ context.Context, accountIDs []int64, _ time.Duration) (map[int64]int64, error) {
	ids := append([]int64(nil), accountIDs...)
	s.beginCalls = append(s.beginCalls, ids)
	if s.onBegin != nil {
		s.onBegin()
	}
	if s.beginErr != nil {
		return nil, s.beginErr
	}
	if s.nextEpoch <= 0 {
		s.nextEpoch = 100
	}
	tokens := make(map[int64]int64, len(ids))
	for _, accountID := range ids {
		s.nextEpoch++
		tokens[accountID] = s.nextEpoch
		if s.accounts != nil {
			delete(s.accounts, accountID)
		}
	}
	return tokens, nil
}

func (s *schedulerMutationCacheRecorder) PublishAccountMutation(_ context.Context, account *service.Account, epoch int64) (bool, error) {
	if account == nil {
		return false, nil
	}
	if s.publishAttempts == nil {
		s.publishAttempts = make(map[int64]int)
	}
	s.publishAttempts[account.ID]++
	if s.publishCalls == nil {
		s.publishCalls = make(map[int64]int64)
	}
	s.publishCalls[account.ID] = epoch
	if s.publishErr != nil {
		return false, s.publishErr
	}
	if s.accounts == nil {
		s.accounts = make(map[int64]*service.Account)
	}
	cloned := *account
	s.accounts[account.ID] = &cloned
	return true, nil
}

func (s *schedulerMutationCacheRecorder) CompleteAccountDeletion(_ context.Context, accountID, epoch int64) (bool, error) {
	if s.completeCalls == nil {
		s.completeCalls = make(map[int64]int64)
	}
	s.completeCalls[accountID] = epoch
	if s.publishErr != nil {
		return false, s.publishErr
	}
	if s.accounts != nil {
		delete(s.accounts, accountID)
	}
	return true, nil
}

func (s *schedulerCacheRecorder) GetSnapshot(ctx context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	return nil, false, nil
}

func (s *schedulerCacheRecorder) SetSnapshot(ctx context.Context, bucket service.SchedulerBucket, accounts []service.Account) error {
	return nil
}

func (s *schedulerCacheRecorder) GetAccount(ctx context.Context, accountID int64) (*service.Account, error) {
	if s.accounts == nil {
		return nil, nil
	}
	return s.accounts[accountID], nil
}

func (s *schedulerCacheRecorder) SetAccount(ctx context.Context, account *service.Account) error {
	s.setAccounts = append(s.setAccounts, account)
	if s.accounts == nil {
		s.accounts = make(map[int64]*service.Account)
	}
	if account != nil {
		s.accounts[account.ID] = account
	}
	return nil
}

func (s *schedulerCacheRecorder) DeleteAccount(ctx context.Context, accountID int64) error {
	s.deleteIDs = append(s.deleteIDs, accountID)
	if s.accounts != nil {
		delete(s.accounts, accountID)
	}
	return nil
}

func (s *schedulerCacheRecorder) UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return nil
}

func (s *schedulerCacheRecorder) TryLockBucket(ctx context.Context, bucket service.SchedulerBucket, ttl time.Duration) (bool, error) {
	return true, nil
}

func (s *schedulerCacheRecorder) UnlockBucket(ctx context.Context, bucket service.SchedulerBucket) error {
	return nil
}

func (s *schedulerCacheRecorder) ListBuckets(ctx context.Context) ([]service.SchedulerBucket, error) {
	return nil, nil
}

func (s *schedulerCacheRecorder) GetOutboxWatermark(ctx context.Context) (int64, error) {
	return 0, nil
}

func (s *schedulerCacheRecorder) SetOutboxWatermark(ctx context.Context, id int64) error {
	return nil
}

func (s *AccountRepoSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.client = tx.Client()
	s.repo = newAccountRepositoryWithSQL(s.client, tx, nil)
}

func TestAccountRepoSuite(t *testing.T) {
	suite.Run(t, new(AccountRepoSuite))
}

func TestAccountRepositoryUpdateWithExtraPatchAmbientRollbackKeepsDBAndCacheClean(t *testing.T) {
	ctx := context.Background()
	account := mustCreateAccount(t, integrationEntClient, &service.Account{
		Name:  "ambient-extra-rollback",
		Extra: map[string]any{"keep": true},
	})
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)

	txCtx := dbent.NewTxContext(ctx, tx)
	cache := &schedulerCacheRecorder{}
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationEntClient, cache)
	name := "ambient-extra-rolled-back"
	require.NoError(t, repo.UpdateWithExtraPatch(txCtx, account.ID, service.AccountColumnPatch{Name: &name}, map[string]any{"mode": "passthrough"}, nil, nil))
	got, err := tx.Client().Account.Get(txCtx, account.ID)
	require.NoError(t, err)
	require.Equal(t, name, got.Name)
	require.Equal(t, true, got.Extra["keep"])
	require.Equal(t, "passthrough", got.Extra["mode"])
	require.Empty(t, cache.setAccounts, "ambient transaction must not publish cache before commit")
	var inTxOutbox int
	require.NoError(t, scanSingleRow(txCtx, tx.Client(), "SELECT COUNT(*) FROM scheduler_outbox WHERE account_id = $1", []any{account.ID}, &inTxOutbox))
	require.Equal(t, 1, inTxOutbox)
	require.NoError(t, tx.Rollback())

	gotAfterRollback, err := integrationEntClient.Account.Get(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, "ambient-extra-rollback", gotAfterRollback.Name)
	require.NotContains(t, gotAfterRollback.Extra, "mode")
	var committedOutbox int
	require.NoError(t, scanSingleRow(ctx, integrationEntClient, "SELECT COUNT(*) FROM scheduler_outbox WHERE account_id = $1", []any{account.ID}, &committedOutbox))
	require.Zero(t, committedOutbox)
}

func TestAccountRepositoryUpdateWithExtraPatchAmbientCommitPersistsOutboxWithoutDirectCache(t *testing.T) {
	ctx := context.Background()
	account := mustCreateAccount(t, integrationEntClient, &service.Account{
		Name:  "ambient-extra-commit",
		Extra: map[string]any{"keep": true},
	})
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(ctx, tx)
	cache := &schedulerCacheRecorder{}
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationEntClient, cache)
	name := "ambient-extra-committed"

	require.NoError(t, repo.UpdateWithExtraPatch(txCtx, account.ID, service.AccountColumnPatch{Name: &name}, map[string]any{"mode": "passthrough"}, nil, nil))
	require.Empty(t, cache.setAccounts)
	require.NoError(t, tx.Commit())

	got, err := integrationEntClient.Account.Get(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, name, got.Name)
	require.Equal(t, "passthrough", got.Extra["mode"])
	var committedOutbox int
	require.NoError(t, scanSingleRow(ctx, integrationEntClient, "SELECT COUNT(*) FROM scheduler_outbox WHERE account_id = $1", []any{account.ID}, &committedOutbox))
	require.Equal(t, 1, committedOutbox)
	require.Empty(t, cache.setAccounts, "ambient commit converges through outbox, not an early direct cache write")
}

func TestAccountRepositoryMutationFenceBeginFailurePreventsDBWrite(t *testing.T) {
	ctx := context.Background()
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "fence-begin-old"})
	beginErr := errors.New("redis unavailable")
	cache := &schedulerMutationCacheRecorder{
		schedulerCacheRecorder: schedulerCacheRecorder{accounts: map[int64]*service.Account{account.ID: account}},
		beginErr:               beginErr,
	}
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationEntClient, cache)
	name := "fence-begin-new"

	err := repo.UpdateWithExtraPatch(ctx, account.ID, service.AccountColumnPatch{Name: &name}, nil, nil, nil)
	require.ErrorIs(t, err, beginErr)
	require.Len(t, cache.beginCalls, 1)
	require.Empty(t, cache.publishCalls)
	got, getErr := integrationEntClient.Account.Get(ctx, account.ID)
	require.NoError(t, getErr)
	require.Equal(t, "fence-begin-old", got.Name)
	var outboxCount int
	require.NoError(t, scanSingleRow(ctx, integrationEntClient,
		"SELECT COUNT(*) FROM scheduler_outbox WHERE account_id = $1", []any{account.ID}, &outboxCount))
	require.Zero(t, outboxCount)
}

func TestAccountRepositoryMutationFenceAmbientRollbackRestoresOldSnapshot(t *testing.T) {
	ctx := context.Background()
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "fence-rollback-old"})
	cache := &schedulerMutationCacheRecorder{
		schedulerCacheRecorder: schedulerCacheRecorder{accounts: map[int64]*service.Account{account.ID: account}},
	}
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationEntClient, cache)
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(ctx, tx)
	name := "fence-rollback-new"

	require.NoError(t, repo.UpdateWithExtraPatch(txCtx, account.ID, service.AccountColumnPatch{Name: &name}, nil, nil, nil))
	require.NotContains(t, cache.accounts, account.ID, "full snapshot must stay missing before outer transaction resolves")
	require.NoError(t, tx.Rollback())
	restored := cache.accounts[account.ID]
	require.NotNil(t, restored)
	require.Equal(t, "fence-rollback-old", restored.Name)
}

func TestAccountRepositoryMutationFenceAmbientCommitPublishesFreshSnapshot(t *testing.T) {
	ctx := context.Background()
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "fence-commit-old"})
	cache := &schedulerMutationCacheRecorder{
		schedulerCacheRecorder: schedulerCacheRecorder{accounts: map[int64]*service.Account{account.ID: account}},
	}
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationEntClient, cache)
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(ctx, tx)
	name := "fence-commit-new"

	require.NoError(t, repo.UpdateWithExtraPatch(txCtx, account.ID, service.AccountColumnPatch{Name: &name}, nil, nil, nil))
	require.NotContains(t, cache.accounts, account.ID)
	require.NoError(t, tx.Commit())
	published := cache.accounts[account.ID]
	require.NotNil(t, published)
	require.Equal(t, "fence-commit-new", published.Name)
	require.Positive(t, cache.publishCalls[account.ID])
}

func TestAccountRepositoryMutationFenceReusesTokenAcrossAmbientAccountAndGroupWrites(t *testing.T) {
	ctx := context.Background()
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "fence-combined-old"})
	group := mustCreateGroup(t, integrationEntClient, &service.Group{Name: "fence-combined-group"})
	cache := &schedulerMutationCacheRecorder{
		schedulerCacheRecorder: schedulerCacheRecorder{accounts: map[int64]*service.Account{account.ID: account}},
	}
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationEntClient, cache)
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(ctx, tx)
	name := "fence-combined-new"

	require.NoError(t, repo.UpdateWithExtraPatch(txCtx, account.ID, service.AccountColumnPatch{Name: &name}, nil, nil, nil))
	require.NoError(t, repo.BindGroups(txCtx, account.ID, []int64{group.ID}))
	require.Len(t, cache.beginCalls, 1, "one outer transaction must reuse the same account token")
	require.NotContains(t, cache.accounts, account.ID)
	require.NoError(t, tx.Commit())

	published := cache.accounts[account.ID]
	require.NotNil(t, published)
	require.Equal(t, name, published.Name)
	require.Equal(t, []int64{group.ID}, published.GroupIDs)
}

func TestAccountRepositoryCreateAndBindGroupsAmbientCommitPublishesOnlyFinalSnapshot(t *testing.T) {
	ctx := context.Background()
	group := mustCreateGroup(t, integrationEntClient, &service.Group{Name: "fence-create-group"})
	cache := &schedulerMutationCacheRecorder{schedulerCacheRecorder: schedulerCacheRecorder{accounts: make(map[int64]*service.Account)}}
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationEntClient, cache)
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(ctx, tx)
	account := &service.Account{
		Name:        "fence-create-account",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{},
		Extra:       map[string]any{},
		Concurrency: 1,
		Priority:    50,
		Status:      service.StatusActive,
		Schedulable: true,
		GroupIDs:    []int64{group.ID},
	}

	require.NoError(t, repo.Create(txCtx, account))
	require.NoError(t, repo.BindGroups(txCtx, account.ID, []int64{group.ID}))
	require.Len(t, cache.beginCalls, 1)
	require.NotContains(t, cache.accounts, account.ID)
	require.NoError(t, tx.Commit())

	published := cache.accounts[account.ID]
	require.NotNil(t, published)
	require.Equal(t, []int64{group.ID}, published.GroupIDs)
	require.Equal(t, 1, cache.publishAttempts[account.ID], "one outer transaction must publish one final snapshot")
}

func TestAccountRepositoryBulkUpdateAndBindGroupsReuseFenceAndPublishFinalSnapshots(t *testing.T) {
	ctx := context.Background()
	group := mustCreateGroup(t, integrationEntClient, &service.Group{Name: "fence-bulk-group"})
	account1 := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "fence-bulk-one"})
	account2 := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "fence-bulk-two"})
	cache := &schedulerMutationCacheRecorder{schedulerCacheRecorder: schedulerCacheRecorder{accounts: map[int64]*service.Account{
		account1.ID: account1,
		account2.ID: account2,
	}}}
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationEntClient, cache)
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(ctx, tx)
	schedulable := false

	updatedIDs, err := repo.BulkUpdateReturningIDs(txCtx, []int64{account1.ID, account2.ID}, service.AccountBulkUpdate{Schedulable: &schedulable})
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{account1.ID, account2.ID}, updatedIDs)
	require.NoError(t, repo.BulkBindGroups(txCtx, updatedIDs, []int64{group.ID}))
	require.Len(t, cache.beginCalls, 1, "bulk fields and groups must reuse one epoch allocation")
	require.NotContains(t, cache.accounts, account1.ID)
	require.NotContains(t, cache.accounts, account2.ID)
	require.NoError(t, tx.Commit())

	for _, accountID := range []int64{account1.ID, account2.ID} {
		published := cache.accounts[accountID]
		require.NotNil(t, published)
		require.False(t, published.Schedulable)
		require.Equal(t, []int64{group.ID}, published.GroupIDs)
		require.Equal(t, 1, cache.publishAttempts[accountID], "each account must publish only its final committed snapshot")
	}
}

func TestGroupRepositoryDirectMembershipWritesUseAccountMutationFence(t *testing.T) {
	ctx := context.Background()
	group := mustCreateGroup(t, integrationEntClient, &service.Group{Name: "group-repo-fence"})
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "group-repo-fenced-account"})
	cache := &schedulerMutationCacheRecorder{schedulerCacheRecorder: schedulerCacheRecorder{accounts: map[int64]*service.Account{
		account.ID: account,
	}}}
	accountMutator := newAccountRepositoryWithSQL(integrationEntClient, integrationEntClient, cache)
	groupRepo := newGroupRepositoryWithSQL(integrationEntClient, integrationEntClient, accountMutator)

	require.NoError(t, groupRepo.BindAccountsToGroup(ctx, group.ID, []int64{account.ID}))
	require.Len(t, cache.beginCalls, 1)
	require.Equal(t, []int64{group.ID}, cache.accounts[account.ID].GroupIDs)

	affected, err := groupRepo.DeleteAccountGroupsByGroupID(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)
	require.Len(t, cache.beginCalls, 2)
	require.Empty(t, cache.accounts[account.ID].GroupIDs)
	require.Equal(t, 2, cache.publishAttempts[account.ID])
}

func TestAccountGroupBatchMutationsSerializeOnGroupRow(t *testing.T) {
	ctx := context.Background()
	group := mustCreateGroup(t, integrationEntClient, &service.Group{Name: "group-membership-lock"})
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "group-membership-lock-account"})
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationEntClient, nil)

	assertWaitsForGroupLock := func(t *testing.T, mutate func() error) {
		t.Helper()
		locker, err := integrationEntClient.Tx(ctx)
		require.NoError(t, err)
		require.NoError(t, lockAccountGroupsForMutation(ctx, locker.Client(), []int64{group.ID}))
		done := make(chan error, 1)
		go func() { done <- mutate() }()
		select {
		case err := <-done:
			_ = locker.Rollback()
			require.Failf(t, "mutation bypassed group lock", "returned early: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
		require.NoError(t, locker.Commit())
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			require.Fail(t, "mutation did not resume after group lock release")
		}
	}

	assertWaitsForGroupLock(t, func() error {
		return repo.BulkAddAccountsToGroup(ctx, group.ID, []int64{account.ID})
	})
	assertWaitsForGroupLock(t, func() error {
		return repo.ReplaceAccountsForGroup(ctx, group.ID, []int64{account.ID})
	})
}

func TestAccountRepositoryMutationFencePublishFailureKeepsCommittedCacheMiss(t *testing.T) {
	ctx := context.Background()
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "fence-publish-old"})
	cache := &schedulerMutationCacheRecorder{
		schedulerCacheRecorder: schedulerCacheRecorder{accounts: map[int64]*service.Account{account.ID: account}},
		publishErr:             errors.New("redis set failed"),
	}
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationEntClient, cache)
	name := "fence-publish-new"

	require.NoError(t, repo.UpdateWithExtraPatch(ctx, account.ID, service.AccountColumnPatch{Name: &name}, nil, nil, nil),
		"post-commit cache failure must not make a committed mutation look retryable")
	got, err := integrationEntClient.Account.Get(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, name, got.Name)
	require.NotContains(t, cache.accounts, account.ID, "failed publish must remain fail-closed")
	var outboxEpoch string
	require.NoError(t, scanSingleRow(ctx, integrationEntClient, `
		SELECT payload->>'scheduler_epoch'
		FROM scheduler_outbox
		WHERE account_id = $1
		ORDER BY id DESC
		LIMIT 1
	`, []any{account.ID}, &outboxEpoch))
	require.NotEmpty(t, outboxEpoch, "transactional outbox must retain the repair token")
}

func TestAccountRepositoryMutationFenceContextCancelRollsBackAndRestores(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "fence-cancel-old"})
	cache := &schedulerMutationCacheRecorder{
		schedulerCacheRecorder: schedulerCacheRecorder{accounts: map[int64]*service.Account{account.ID: account}},
		onBegin:                cancel,
	}
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationEntClient, cache)
	name := "fence-cancel-new"

	err := repo.UpdateWithExtraPatch(ctx, account.ID, service.AccountColumnPatch{Name: &name}, nil, nil, nil)
	require.Error(t, err)
	restored := cache.accounts[account.ID]
	require.NotNil(t, restored)
	require.Equal(t, "fence-cancel-old", restored.Name)
}

func TestAccountRepositoryMutationFenceRejectsImplicitTransactionalClient(t *testing.T) {
	ctx := context.Background()
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "fence-implicit-old"})
	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	cache := &schedulerMutationCacheRecorder{}
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, cache)
	name := "fence-implicit-new"

	err = repo.UpdateWithExtraPatch(ctx, account.ID, service.AccountColumnPatch{Name: &name}, nil, nil, nil)
	require.ErrorIs(t, err, errSchedulerMutationTxContextRequired)
	require.Empty(t, cache.beginCalls, "write must be rejected before Redis or DB mutation")
	got, getErr := tx.Client().Account.Get(ctx, account.ID)
	require.NoError(t, getErr)
	require.Equal(t, "fence-implicit-old", got.Name)
}

// --- Create / GetByID / Update / Delete ---

func (s *AccountRepoSuite) TestCreate() {
	tempUntil := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Second)
	tempReason := "temporary upstream failure"
	account := &service.Account{
		Name:                    "test-create",
		Platform:                service.PlatformAnthropic,
		Type:                    service.AccountTypeOAuth,
		Status:                  service.StatusActive,
		Credentials:             map[string]any{},
		Extra:                   map[string]any{},
		Concurrency:             3,
		Priority:                50,
		Schedulable:             true,
		TempUnschedulableUntil:  &tempUntil,
		TempUnschedulableReason: tempReason,
	}

	err := s.repo.Create(s.ctx, account)
	s.Require().NoError(err, "Create")
	s.Require().NotZero(account.ID, "expected ID to be set")

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal("test-create", got.Name)
	s.Require().NotNil(got.TempUnschedulableUntil)
	s.Require().WithinDuration(tempUntil, *got.TempUnschedulableUntil, time.Second)
	s.Require().Equal(tempReason, got.TempUnschedulableReason)
}

func (s *AccountRepoSuite) TestGetByID_NotFound() {
	_, err := s.repo.GetByID(s.ctx, 999999)
	s.Require().Error(err, "expected error for non-existent ID")
}

func (s *AccountRepoSuite) TestUpdate() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "original"})
	tempUntil := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Second)
	tempReason := "temporary transport failure"

	account.Name = "updated"
	account.TempUnschedulableUntil = &tempUntil
	account.TempUnschedulableReason = tempReason
	err := s.repo.Update(s.ctx, account)
	s.Require().NoError(err, "Update")

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("updated", got.Name)
	s.Require().NotNil(got.TempUnschedulableUntil)
	s.Require().WithinDuration(tempUntil, *got.TempUnschedulableUntil, time.Second)
	s.Require().Equal(tempReason, got.TempUnschedulableReason)

	got.TempUnschedulableUntil = nil
	got.TempUnschedulableReason = ""
	s.Require().NoError(s.repo.Update(s.ctx, got), "clear temporary state")
	cleared, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID after clearing temporary state")
	s.Require().Nil(cleared.TempUnschedulableUntil)
	s.Require().Empty(cleared.TempUnschedulableReason)
}

func (s *AccountRepoSuite) TestUpdate_SyncSchedulerSnapshotOnDisabled() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "sync-update", Status: service.StatusActive, Schedulable: true})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	account.Status = service.StatusDisabled
	err := s.repo.Update(s.ctx, account)
	s.Require().NoError(err, "Update")

	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Equal(service.StatusDisabled, cacheRecorder.setAccounts[0].Status)
}

func (s *AccountRepoSuite) TestUpdate_SyncSchedulerSnapshotOnCredentialsChange() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "sync-credentials-update",
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gpt-5": "gpt-5.1",
			},
		},
	})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	account.Credentials = map[string]any{
		"model_mapping": map[string]any{
			"gpt-5": "gpt-5.2",
		},
	}
	err := s.repo.Update(s.ctx, account)
	s.Require().NoError(err, "Update")

	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	mapping, ok := cacheRecorder.setAccounts[0].Credentials["model_mapping"].(map[string]any)
	s.Require().True(ok)
	s.Require().Equal("gpt-5.2", mapping["gpt-5"])
}

func (s *AccountRepoSuite) TestDelete() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "to-delete"})

	err := s.repo.Delete(s.ctx, account.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, account.ID)
	s.Require().Error(err, "expected error after delete")
}

func (s *AccountRepoSuite) TestDelete_RemovesSchedulerAccountSnapshot() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "to-delete-cache"})
	cacheRecorder := &schedulerCacheRecorder{
		accounts: map[int64]*service.Account{
			account.ID: {
				ID:          account.ID,
				Name:        account.Name,
				Status:      service.StatusActive,
				Schedulable: true,
			},
		},
	}
	s.repo.schedulerCache = cacheRecorder

	err := s.repo.Delete(s.ctx, account.ID)
	s.Require().NoError(err, "Delete")

	s.Require().Equal([]int64{account.ID}, cacheRecorder.deleteIDs)
	s.Require().NotContains(cacheRecorder.accounts, account.ID)
}

func (s *AccountRepoSuite) TestDelete_WithGroupBindings() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-del"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-del"})
	mustBindAccountToGroup(s.T(), s.client, account.ID, group.ID, 1)

	err := s.repo.Delete(s.ctx, account.ID)
	s.Require().NoError(err, "Delete should cascade remove bindings")

	count, err := s.client.AccountGroup.Query().Where(accountgroup.AccountIDEQ(account.ID)).Count(s.ctx)
	s.Require().NoError(err)
	s.Require().Zero(count, "expected bindings to be removed")
}

// --- List / ListWithFilters ---

func (s *AccountRepoSuite) TestList() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc1"})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc2"})

	accounts, page, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "List")
	s.Require().Len(accounts, 2)
	s.Require().Equal(int64(2), page.Total)
}

func (s *AccountRepoSuite) TestListWithFilters() {
	tests := []struct {
		name        string
		setup       func(client *dbent.Client)
		platform    string
		accType     string
		status      string
		search      string
		groupID     int64
		privacyMode string
		wantCount   int
		validate    func(accounts []service.Account)
	}{
		{
			name: "filter_by_platform",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "a1", Platform: service.PlatformAnthropic})
				mustCreateAccount(s.T(), client, &service.Account{Name: "a2", Platform: service.PlatformOpenAI})
			},
			platform:  service.PlatformOpenAI,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal(service.PlatformOpenAI, accounts[0].Platform)
			},
		},
		{
			name: "filter_by_type",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "t1", Type: service.AccountTypeOAuth})
				mustCreateAccount(s.T(), client, &service.Account{Name: "t2", Type: service.AccountTypeAPIKey})
			},
			accType:   service.AccountTypeAPIKey,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal(service.AccountTypeAPIKey, accounts[0].Type)
			},
		},
		{
			name: "filter_by_status",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "s1", Status: service.StatusActive})
				mustCreateAccount(s.T(), client, &service.Account{Name: "s2", Status: service.StatusDisabled})
			},
			status:    service.StatusDisabled,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal(service.StatusDisabled, accounts[0].Status)
			},
		},
		{
			name: "filter_by_status_active_excludes_runtime_blocked_accounts",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "active-normal", Status: service.StatusActive})
				rateLimited := mustCreateAccount(s.T(), client, &service.Account{Name: "active-rate-limited", Status: service.StatusActive})
				err := client.Account.UpdateOneID(rateLimited.ID).
					SetRateLimitResetAt(time.Now().Add(10 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActive})
				err = client.Account.UpdateOneID(tempUnsched.ID).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				unsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-unsched", Status: service.StatusActive})
				err = client.Account.UpdateOneID(unsched.ID).
					SetSchedulable(false).
					Exec(context.Background())
				s.Require().NoError(err)
			},
			status:    service.StatusActive,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-normal", accounts[0].Name)
			},
		},
		{
			name: "filter_by_status_active_includes_pool_mode_with_rate_limit_timestamp",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "active-normal", Status: service.StatusActive})
				poolMode := mustCreateAccount(s.T(), client, &service.Account{
					Name:        "pool-mode-active",
					Status:      service.StatusActive,
					Type:        service.AccountTypeAPIKey,
					Platform:    service.PlatformOpenAI,
					Credentials: map[string]any{"pool_mode": true},
				})
				err := client.Account.UpdateOneID(poolMode.ID).
					SetRateLimitResetAt(time.Now().Add(10 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
			},
			status:    service.StatusActive,
			wantCount: 2,
			validate: func(accounts []service.Account) {
				names := []string{accounts[0].Name, accounts[1].Name}
				s.ElementsMatch([]string{"active-normal", "pool-mode-active"}, names)
			},
		},
		{
			name: "filter_by_status_unschedulable_excludes_rate_limited_and_temp_unschedulable",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "active-normal", Status: service.StatusActive, Schedulable: true})
				unsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-unsched", Status: service.StatusActive})
				err := client.Account.UpdateOneID(unsched.ID).
					SetSchedulable(false).
					Exec(context.Background())
				s.Require().NoError(err)
				rateLimited := mustCreateAccount(s.T(), client, &service.Account{Name: "active-rate-limited", Status: service.StatusActive})
				err = client.Account.UpdateOneID(rateLimited.ID).
					SetSchedulable(false).
					SetRateLimitResetAt(time.Now().Add(10 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActive})
				err = client.Account.UpdateOneID(tempUnsched.ID).
					SetSchedulable(false).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
			},
			status:    "unschedulable",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-unsched", accounts[0].Name)
			},
		},
		{
			name: "filter_by_status_rate_limited_excludes_temp_unschedulable",
			setup: func(client *dbent.Client) {
				rateLimited := mustCreateAccount(s.T(), client, &service.Account{Name: "active-rate-limited", Status: service.StatusActive})
				err := client.Account.UpdateOneID(rateLimited.ID).
					SetRateLimitResetAt(time.Now().Add(10 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActive})
				err = client.Account.UpdateOneID(tempUnsched.ID).
					SetRateLimitResetAt(time.Now().Add(20 * time.Minute)).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
			},
			status:    "rate_limited",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-rate-limited", accounts[0].Name)
			},
		},
		{
			name: "filter_by_status_rate_limited_excludes_pool_mode_with_rate_limit_timestamp",
			setup: func(client *dbent.Client) {
				rateLimited := mustCreateAccount(s.T(), client, &service.Account{Name: "normal-rate-limited", Status: service.StatusActive})
				poolMode := mustCreateAccount(s.T(), client, &service.Account{
					Name:        "pool-mode-rate-limit-ts",
					Status:      service.StatusActive,
					Type:        service.AccountTypeAPIKey,
					Platform:    service.PlatformOpenAI,
					Credentials: map[string]any{"pool_mode": true},
				})
				err := client.Account.UpdateOneID(rateLimited.ID).
					SetRateLimitResetAt(time.Now().Add(10 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				err = client.Account.UpdateOneID(poolMode.ID).
					SetRateLimitResetAt(time.Now().Add(10 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
			},
			status:    "rate_limited",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("normal-rate-limited", accounts[0].Name)
			},
		},
		{
			name: "filter_by_status_temp_unschedulable_excludes_manually_unschedulable",
			setup: func(client *dbent.Client) {
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActive, Schedulable: true})
				err := client.Account.UpdateOneID(tempUnsched.ID).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				unsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-unsched", Status: service.StatusActive})
				err = client.Account.UpdateOneID(unsched.ID).
					SetSchedulable(false).
					Exec(context.Background())
				s.Require().NoError(err)
			},
			status:    "temp_unschedulable",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-temp-unsched", accounts[0].Name)
			},
		},
		{
			name: "filter_by_search",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "alpha-account"})
				mustCreateAccount(s.T(), client, &service.Account{Name: "beta-account"})
			},
			search:    "alpha",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Contains(accounts[0].Name, "alpha")
			},
		},
		{
			name: "filter_by_ungrouped",
			setup: func(client *dbent.Client) {
				group := mustCreateGroup(s.T(), client, &service.Group{Name: "g-ungrouped"})
				grouped := mustCreateAccount(s.T(), client, &service.Account{Name: "grouped-account"})
				mustCreateAccount(s.T(), client, &service.Account{Name: "ungrouped-account"})
				mustBindAccountToGroup(s.T(), client, grouped.ID, group.ID, 1)
			},
			groupID:   service.AccountListGroupUngrouped,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("ungrouped-account", accounts[0].Name)
				s.Require().Empty(accounts[0].GroupIDs)
			},
		},
		{
			name: "filter_by_privacy_mode",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-ok", Extra: map[string]any{"privacy_mode": service.PrivacyModeTrainingOff}})
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-fail", Extra: map[string]any{"privacy_mode": service.PrivacyModeFailed}})
			},
			privacyMode: service.PrivacyModeTrainingOff,
			wantCount:   1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("privacy-ok", accounts[0].Name)
			},
		},
		{
			name: "filter_by_privacy_mode_unset",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-unset", Extra: nil})
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-empty", Extra: map[string]any{"privacy_mode": ""}})
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-set", Extra: map[string]any{"privacy_mode": service.PrivacyModeTrainingOff}})
			},
			privacyMode: service.AccountPrivacyModeUnsetFilter,
			wantCount:   2,
			validate: func(accounts []service.Account) {
				names := []string{accounts[0].Name, accounts[1].Name}
				s.ElementsMatch([]string{"privacy-unset", "privacy-empty"}, names)
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// 每个 case 重新获取隔离资源
			tx := testEntTx(s.T())
			client := tx.Client()
			repo := newAccountRepositoryWithSQL(client, tx, nil)
			ctx := context.Background()

			tt.setup(client)

			accounts, _, err := repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, tt.platform, tt.accType, tt.status, tt.search, tt.groupID, tt.privacyMode)
			s.Require().NoError(err)
			s.Require().Len(accounts, tt.wantCount)
			if tt.validate != nil {
				tt.validate(accounts)
			}
		})
	}
}

// --- ListByGroup / ListActive / ListByPlatform ---

func (s *AccountRepoSuite) TestListByGroup() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-list"})
	acc1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a1", Status: service.StatusActive})
	acc2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a2", Status: service.StatusActive})
	mustBindAccountToGroup(s.T(), s.client, acc1.ID, group.ID, 2)
	mustBindAccountToGroup(s.T(), s.client, acc2.ID, group.ID, 1)

	accounts, err := s.repo.ListByGroup(s.ctx, group.ID)
	s.Require().NoError(err, "ListByGroup")
	s.Require().Len(accounts, 2)
	// Should be ordered by priority
	s.Require().Equal(acc2.ID, accounts[0].ID, "expected acc2 first (priority=1)")
}

func (s *AccountRepoSuite) TestListActive() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "active1", Status: service.StatusActive})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "inactive1", Status: service.StatusDisabled})

	accounts, err := s.repo.ListActive(s.ctx)
	s.Require().NoError(err, "ListActive")
	s.Require().Len(accounts, 1)
	s.Require().Equal("active1", accounts[0].Name)
}

func (s *AccountRepoSuite) TestListByPlatform() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "p1", Platform: service.PlatformAnthropic, Status: service.StatusActive})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "p2", Platform: service.PlatformOpenAI, Status: service.StatusActive})

	accounts, err := s.repo.ListByPlatform(s.ctx, service.PlatformAnthropic)
	s.Require().NoError(err, "ListByPlatform")
	s.Require().Len(accounts, 1)
	s.Require().Equal(service.PlatformAnthropic, accounts[0].Platform)
}

// --- Preload and VirtualFields ---

func (s *AccountRepoSuite) TestPreload_And_VirtualFields() {
	proxy := mustCreateProxy(s.T(), s.client, &service.Proxy{Name: "p1"})
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g1"})

	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:    "acc1",
		ProxyID: &proxy.ID,
	})
	mustBindAccountToGroup(s.T(), s.client, account.ID, group.ID, 1)

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().NotNil(got.Proxy, "expected Proxy preload")
	s.Require().Equal(proxy.ID, got.Proxy.ID)
	s.Require().Len(got.GroupIDs, 1, "expected GroupIDs to be populated")
	s.Require().Equal(group.ID, got.GroupIDs[0])
	s.Require().Len(got.Groups, 1, "expected Groups to be populated")
	s.Require().Equal(group.ID, got.Groups[0].ID)

	accounts, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", "", "", "acc", 0, "")
	s.Require().NoError(err, "ListWithFilters")
	s.Require().Equal(int64(1), page.Total)
	s.Require().Len(accounts, 1)
	s.Require().NotNil(accounts[0].Proxy, "expected Proxy preload in list")
	s.Require().Equal(proxy.ID, accounts[0].Proxy.ID)
	s.Require().Len(accounts[0].GroupIDs, 1, "expected GroupIDs in list")
	s.Require().Equal(group.ID, accounts[0].GroupIDs[0])
}

// --- GroupBinding / AddToGroup / RemoveFromGroup / BindGroups / GetGroups ---

func (s *AccountRepoSuite) TestGroupBinding_And_BindGroups() {
	g1 := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g1"})
	g2 := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g2"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc"})

	s.Require().NoError(s.repo.AddToGroup(s.ctx, account.ID, g1.ID, 10), "AddToGroup")
	groups, err := s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err, "GetGroups")
	s.Require().Len(groups, 1, "expected 1 group")
	s.Require().Equal(g1.ID, groups[0].ID)

	s.Require().NoError(s.repo.RemoveFromGroup(s.ctx, account.ID, g1.ID), "RemoveFromGroup")
	groups, err = s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err, "GetGroups after remove")
	s.Require().Empty(groups, "expected 0 groups after remove")

	s.Require().NoError(s.repo.BindGroups(s.ctx, account.ID, []int64{g1.ID, g2.ID}), "BindGroups")
	groups, err = s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err, "GetGroups after bind")
	s.Require().Len(groups, 2, "expected 2 groups after bind")
}

func (s *AccountRepoSuite) TestBindGroups_EmptyList() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-empty"})
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-empty"})
	mustBindAccountToGroup(s.T(), s.client, account.ID, group.ID, 1)
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.BindGroups(s.ctx, account.ID, []int64{}), "BindGroups empty")

	groups, err := s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Empty(groups, "expected 0 groups after binding empty list")
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Empty(cacheRecorder.setAccounts[0].GroupIDs)
}

// --- Schedulable ---

func (s *AccountRepoSuite) TestListSchedulable() {
	now := time.Now()
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-sched"})

	okAcc := mustCreateAccount(s.T(), s.client, &service.Account{Name: "ok", Schedulable: true})
	mustBindAccountToGroup(s.T(), s.client, okAcc.ID, group.ID, 1)

	future := now.Add(10 * time.Minute)
	overloaded := mustCreateAccount(s.T(), s.client, &service.Account{Name: "over", Schedulable: true, OverloadUntil: &future})
	mustBindAccountToGroup(s.T(), s.client, overloaded.ID, group.ID, 1)
	tempUnschedulable := mustCreateAccount(s.T(), s.client, &service.Account{Name: "temp", Schedulable: true, TempUnschedulableUntil: &future})
	mustBindAccountToGroup(s.T(), s.client, tempUnschedulable.ID, group.ID, 1)

	poolModeOverloaded := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:                    "pool-over",
		Schedulable:             true,
		Status:                  service.StatusError,
		Type:                    service.AccountTypeAPIKey,
		Platform:                service.PlatformOpenAI,
		Credentials:             map[string]any{"pool_mode": "true"},
		OverloadUntil:           &future,
		TempUnschedulableUntil:  &future,
		TempUnschedulableReason: "stale pool state",
	})
	mustBindAccountToGroup(s.T(), s.client, poolModeOverloaded.ID, group.ID, 1)

	sched, err := s.repo.ListSchedulable(s.ctx)
	s.Require().NoError(err, "ListSchedulable")
	ids := idsOfAccounts(sched)
	s.Require().Contains(ids, okAcc.ID)
	s.Require().Contains(ids, poolModeOverloaded.ID)
	s.Require().NotContains(ids, overloaded.ID)
	s.Require().NotContains(ids, tempUnschedulable.ID)
}

func (s *AccountRepoSuite) TestListWithFilters_PoolModeErrorExcludedFromErrorStatus() {
	poolModeErr := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:         "pool-err",
		Status:       service.StatusError,
		ErrorMessage: "boom",
		Type:         service.AccountTypeAPIKey,
		Platform:     service.PlatformOpenAI,
		Credentials:  map[string]any{"pool_mode": true},
	})
	normalErr := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:         "normal-err",
		Status:       service.StatusError,
		ErrorMessage: "boom",
	})

	accounts, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", "", service.StatusError, "", 0, "")
	s.Require().NoError(err)
	names := make([]string, 0, len(accounts))
	for _, account := range accounts {
		names = append(names, account.Name)
	}
	s.Require().Contains(names, normalErr.Name)
	s.Require().NotContains(names, poolModeErr.Name)
}

func (s *AccountRepoSuite) TestListWithFilters_PoolModeErrorIncludedInActiveStatus() {
	poolModeErr := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:         "pool-err-active",
		Status:       service.StatusError,
		ErrorMessage: "boom",
		Type:         service.AccountTypeAPIKey,
		Platform:     service.PlatformOpenAI,
		Credentials:  map[string]any{"pool_mode": true},
	})

	accounts, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", "", service.StatusActive, "", 0, "")
	s.Require().NoError(err)
	names := make([]string, 0, len(accounts))
	for _, account := range accounts {
		names = append(names, account.Name)
	}
	s.Require().Contains(names, poolModeErr.Name)
}

func (s *AccountRepoSuite) TestListWithFilters_PoolModeOverloadedIncludedInActiveStatus() {
	future := time.Now().Add(10 * time.Minute)
	poolModeOverloaded := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:          "pool-over-active",
		Status:        service.StatusActive,
		Schedulable:   true,
		Type:          service.AccountTypeAPIKey,
		Platform:      service.PlatformOpenAI,
		Credentials:   map[string]any{"pool_mode": true},
		OverloadUntil: &future,
	})

	accounts, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "", "", service.StatusActive, "", 0, "")
	s.Require().NoError(err)
	names := make([]string, 0, len(accounts))
	for _, account := range accounts {
		names = append(names, account.Name)
	}
	s.Require().Contains(names, poolModeOverloaded.Name)
}

func (s *AccountRepoSuite) TestListSchedulableByPlatform_PoolModeErrorIncludedReal() {
	poolModeErr := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "pool-platform-err",
		Status:      service.StatusError,
		Schedulable: true,
		Type:        service.AccountTypeAPIKey,
		Platform:    service.PlatformOpenAI,
		Credentials: map[string]any{"pool_mode": true},
	})

	accounts, err := s.repo.ListSchedulableByPlatform(s.ctx, service.PlatformOpenAI)
	s.Require().NoError(err)
	ids := idsOfAccounts(accounts)
	s.Require().Contains(ids, poolModeErr.ID)
}

func (s *AccountRepoSuite) TestListSchedulableUngroupedByPlatform_PoolModeErrorIncluded() {
	poolModeErr := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "pool-ungrouped-err",
		Status:      service.StatusError,
		Schedulable: true,
		Type:        service.AccountTypeAPIKey,
		Platform:    service.PlatformOpenAI,
		Credentials: map[string]any{"pool_mode": true},
	})

	accounts, err := s.repo.ListSchedulableUngroupedByPlatform(s.ctx, service.PlatformOpenAI)
	s.Require().NoError(err)
	ids := idsOfAccounts(accounts)
	s.Require().Contains(ids, poolModeErr.ID)
}

func (s *AccountRepoSuite) TestListSchedulableByPlatforms_PoolModeErrorIncluded() {
	poolModeErr := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "pool-platforms-err",
		Status:      service.StatusError,
		Schedulable: true,
		Type:        service.AccountTypeAPIKey,
		Platform:    service.PlatformOpenAI,
		Credentials: map[string]any{"pool_mode": true},
	})

	accounts, err := s.repo.ListSchedulableByPlatforms(s.ctx, []string{service.PlatformOpenAI})
	s.Require().NoError(err)
	ids := idsOfAccounts(accounts)
	s.Require().Contains(ids, poolModeErr.ID)
}

func (s *AccountRepoSuite) TestListSchedulableUngroupedByPlatforms_PoolModeErrorIncluded() {
	poolModeErr := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "pool-ungrouped-platforms-err",
		Status:      service.StatusError,
		Schedulable: true,
		Type:        service.AccountTypeAPIKey,
		Platform:    service.PlatformOpenAI,
		Credentials: map[string]any{"pool_mode": true},
	})

	accounts, err := s.repo.ListSchedulableUngroupedByPlatforms(s.ctx, []string{service.PlatformOpenAI})
	s.Require().NoError(err)
	ids := idsOfAccounts(accounts)
	s.Require().Contains(ids, poolModeErr.ID)
}

func (s *AccountRepoSuite) TestListSchedulableByGroupID_TimeBoundaries_And_StatusUpdates() {
	now := time.Now()
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-sched"})

	okAcc := mustCreateAccount(s.T(), s.client, &service.Account{Name: "ok", Schedulable: true})
	mustBindAccountToGroup(s.T(), s.client, okAcc.ID, group.ID, 1)

	future := now.Add(10 * time.Minute)
	overloaded := mustCreateAccount(s.T(), s.client, &service.Account{Name: "over", Schedulable: true, OverloadUntil: &future})
	mustBindAccountToGroup(s.T(), s.client, overloaded.ID, group.ID, 1)

	rateLimited := mustCreateAccount(s.T(), s.client, &service.Account{Name: "rl", Schedulable: true})
	mustBindAccountToGroup(s.T(), s.client, rateLimited.ID, group.ID, 1)
	s.Require().NoError(s.repo.SetRateLimited(s.ctx, rateLimited.ID, now.Add(10*time.Minute)), "SetRateLimited")

	s.Require().NoError(s.repo.SetError(s.ctx, overloaded.ID, "boom"), "SetError")

	sched, err := s.repo.ListSchedulableByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "ListSchedulableByGroupID")
	s.Require().Len(sched, 1, "expected only ok account schedulable")
	s.Require().Equal(okAcc.ID, sched[0].ID)

	s.Require().NoError(s.repo.ClearRateLimit(s.ctx, rateLimited.ID), "ClearRateLimit")
	sched2, err := s.repo.ListSchedulableByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "ListSchedulableByGroupID after ClearRateLimit")
	s.Require().Len(sched2, 2, "expected 2 schedulable accounts after ClearRateLimit")
}

func (s *AccountRepoSuite) TestListSchedulableByPlatform() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "a1", Platform: service.PlatformAnthropic, Schedulable: true})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "a2", Platform: service.PlatformOpenAI, Schedulable: true})

	accounts, err := s.repo.ListSchedulableByPlatform(s.ctx, service.PlatformAnthropic)
	s.Require().NoError(err)
	s.Require().Len(accounts, 1)
	s.Require().Equal(service.PlatformAnthropic, accounts[0].Platform)
}

func (s *AccountRepoSuite) TestListSchedulableByGroupIDAndPlatform() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-sp"})
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a1", Platform: service.PlatformAnthropic, Schedulable: true})
	a2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a2", Platform: service.PlatformOpenAI, Schedulable: true})
	mustBindAccountToGroup(s.T(), s.client, a1.ID, group.ID, 1)
	mustBindAccountToGroup(s.T(), s.client, a2.ID, group.ID, 2)

	accounts, err := s.repo.ListSchedulableByGroupIDAndPlatform(s.ctx, group.ID, service.PlatformAnthropic)
	s.Require().NoError(err)
	s.Require().Len(accounts, 1)
	s.Require().Equal(a1.ID, accounts[0].ID)
}

func (s *AccountRepoSuite) TestSetSchedulable() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-sched", Schedulable: true})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.SetSchedulable(s.ctx, account.ID, false))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().False(got.Schedulable)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
}

func (s *AccountRepoSuite) TestBulkUpdate_SyncSchedulerSnapshotOnDisabled() {
	account1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-1", Status: service.StatusActive, Schedulable: true})
	account2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-2", Status: service.StatusActive, Schedulable: true})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	disabled := service.StatusDisabled
	rows, err := s.repo.BulkUpdate(s.ctx, []int64{account1.ID, account2.ID}, service.AccountBulkUpdate{
		Status: &disabled,
	})
	s.Require().NoError(err)
	s.Require().Equal(int64(2), rows)

	s.Require().Len(cacheRecorder.setAccounts, 2)
	ids := map[int64]struct{}{}
	for _, acc := range cacheRecorder.setAccounts {
		ids[acc.ID] = struct{}{}
	}
	s.Require().Contains(ids, account1.ID)
	s.Require().Contains(ids, account2.ID)
}

func (s *AccountRepoSuite) TestBulkUpdate_SyncSchedulerSnapshotOnRoutingChange() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-routing", Status: service.StatusActive, Schedulable: true})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	rows, err := s.repo.BulkUpdate(s.ctx, []int64{account.ID}, service.AccountBulkUpdate{
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5": "gpt-5-codex"}},
	})
	s.Require().NoError(err)
	s.Require().Equal(int64(1), rows)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Equal("gpt-5-codex", cacheRecorder.setAccounts[0].GetMappedModel("gpt-5"))
}

// --- SetOverloaded / SetRateLimited / ClearRateLimit ---

func (s *AccountRepoSuite) TestSetOverloaded() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-over"})
	until := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.SetOverloaded(s.ctx, account.ID, until))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.OverloadUntil)
	s.Require().WithinDuration(until, *got.OverloadUntil, time.Second)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().NotNil(cacheRecorder.setAccounts[0].OverloadUntil)
	s.Require().WithinDuration(until, *cacheRecorder.setAccounts[0].OverloadUntil, time.Second)
}

func (s *AccountRepoSuite) TestSetRateLimited() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-rl"})
	resetAt := time.Date(2025, 6, 15, 14, 0, 0, 0, time.UTC)

	s.Require().NoError(s.repo.SetRateLimited(s.ctx, account.ID, resetAt))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.RateLimitedAt)
	s.Require().NotNil(got.RateLimitResetAt)
	s.Require().WithinDuration(resetAt, *got.RateLimitResetAt, time.Second)
}

func (s *AccountRepoSuite) TestClearRateLimit() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-clear"})
	until := time.Now().Add(1 * time.Hour)
	s.Require().NoError(s.repo.SetOverloaded(s.ctx, account.ID, until))
	s.Require().NoError(s.repo.SetRateLimited(s.ctx, account.ID, until))

	s.Require().NoError(s.repo.ClearRateLimit(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Nil(got.RateLimitedAt)
	s.Require().Nil(got.RateLimitResetAt)
	s.Require().Nil(got.OverloadUntil)
}

func (s *AccountRepoSuite) TestTempUnschedulableFieldsLoadedByGetByIDAndGetByIDs() {
	acc1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-temp-1"})
	acc2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-temp-2"})

	until := time.Now().Add(15 * time.Minute).UTC().Truncate(time.Second)
	reason := `{"rule":"429","matched_keyword":"too many requests"}`
	s.Require().NoError(s.repo.SetTempUnschedulable(s.ctx, acc1.ID, until, reason))

	gotByID, err := s.repo.GetByID(s.ctx, acc1.ID)
	s.Require().NoError(err)
	s.Require().NotNil(gotByID.TempUnschedulableUntil)
	s.Require().WithinDuration(until, *gotByID.TempUnschedulableUntil, time.Second)
	s.Require().Equal(reason, gotByID.TempUnschedulableReason)

	gotByIDs, err := s.repo.GetByIDs(s.ctx, []int64{acc2.ID, acc1.ID})
	s.Require().NoError(err)
	s.Require().Len(gotByIDs, 2)
	s.Require().Equal(acc2.ID, gotByIDs[0].ID)
	s.Require().Nil(gotByIDs[0].TempUnschedulableUntil)
	s.Require().Equal("", gotByIDs[0].TempUnschedulableReason)
	s.Require().Equal(acc1.ID, gotByIDs[1].ID)
	s.Require().NotNil(gotByIDs[1].TempUnschedulableUntil)
	s.Require().WithinDuration(until, *gotByIDs[1].TempUnschedulableUntil, time.Second)
	s.Require().Equal(reason, gotByIDs[1].TempUnschedulableReason)

	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.ClearTempUnschedulable(s.ctx, acc1.ID))
	cleared, err := s.repo.GetByID(s.ctx, acc1.ID)
	s.Require().NoError(err)
	s.Require().Nil(cleared.TempUnschedulableUntil)
	s.Require().Equal("", cleared.TempUnschedulableReason)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(acc1.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Nil(cacheRecorder.setAccounts[0].TempUnschedulableUntil)
	s.Require().Equal("", cacheRecorder.setAccounts[0].TempUnschedulableReason)
}

func (s *AccountRepoSuite) TestClearModelRateLimits_SyncsSchedulerSnapshot() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "acc-clear-model-rate",
		Extra: map[string]any{
			"model_rate_limits": map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limit_reset_at": "2026-06-03T10:00:00Z",
				},
			},
		},
	})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.ClearModelRateLimits(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotContains(got.Extra, "model_rate_limits")
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().NotContains(cacheRecorder.setAccounts[0].Extra, "model_rate_limits")
}

// --- UpdateLastUsed ---

func (s *AccountRepoSuite) TestUpdateLastUsed() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-used"})
	s.Require().Nil(account.LastUsedAt)

	s.Require().NoError(s.repo.UpdateLastUsed(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.LastUsedAt)
}

// --- SetError ---

func (s *AccountRepoSuite) TestSetError() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-err", Status: service.StatusActive, Schedulable: true})

	s.Require().NoError(s.repo.SetError(s.ctx, account.ID, "something went wrong"))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusError, got.Status)
	s.Require().Equal("something went wrong", got.ErrorMessage)
	s.Require().False(got.Schedulable)
}

func (s *AccountRepoSuite) TestUpdateErrorStatusUnschedulesAccount() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-update-err", Status: service.StatusActive, Schedulable: true})
	account.Status = service.StatusError
	account.ErrorMessage = "token revoked"
	account.Schedulable = true

	s.Require().NoError(s.repo.Update(s.ctx, account))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusError, got.Status)
	s.Require().Equal("token revoked", got.ErrorMessage)
	s.Require().False(got.Schedulable)
}

func (s *AccountRepoSuite) TestClearError_SyncSchedulerSnapshotOnRecovery() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:         "acc-clear-err",
		Status:       service.StatusError,
		ErrorMessage: "temporary error",
	})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.ClearError(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusActive, got.Status)
	s.Require().Empty(got.ErrorMessage)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Equal(service.StatusActive, cacheRecorder.setAccounts[0].Status)
}

// --- UpdateSessionWindow ---

func (s *AccountRepoSuite) TestUpdateSessionWindow() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-win"})
	start := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 6, 15, 15, 0, 0, 0, time.UTC)

	s.Require().NoError(s.repo.UpdateSessionWindow(s.ctx, account.ID, &start, &end, "active"))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.SessionWindowStart)
	s.Require().NotNil(got.SessionWindowEnd)
	s.Require().Equal("active", got.SessionWindowStatus)
}

// --- UpdateExtra ---

func (s *AccountRepoSuite) TestUpdateExtra_MergesFields() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:  "acc-extra",
		Extra: map[string]any{"a": "1"},
	})
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{"b": "2"}), "UpdateExtra")

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal("1", got.Extra["a"])
	s.Require().Equal("2", got.Extra["b"])
}

func (s *AccountRepoSuite) TestUpdateExtra_EmptyUpdates() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-extra-empty"})
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{}))
}

func (s *AccountRepoSuite) TestUpdateExtra_NilExtra() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-nil-extra", Extra: nil})
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{"key": "val"}))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("val", got.Extra["key"])
}

func (s *AccountRepoSuite) TestUpdateWithExtraPatchPreservesConcurrentRuntimeAndNestedFields() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "acc-extra-patch",
		Extra: map[string]any{
			"legacy": "remove",
			"aether_ws": map[string]any{
				"schema_version": 1,
				"enabled":        false,
				"future_field":   "preserve",
			},
		},
	})
	stale, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)

	// Simulates runtime state arriving after the admin loaded the form. An
	// Aether-only patch must not write any of this stale account snapshot back.
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{
		"passive_usage_sampled_at": "2026-07-15T00:00:00Z",
	}))
	lastUsedAt := time.Date(2026, 7, 15, 1, 2, 3, 0, time.UTC)
	rateLimitResetAt := lastUsedAt.Add(time.Hour)
	_, err = s.client.Account.UpdateOneID(account.ID).
		SetCredentials(map[string]any{"runtime_token": "new-token"}).
		SetStatus(service.StatusError).
		SetSchedulable(false).
		SetLastUsedAt(lastUsedAt).
		SetRateLimitResetAt(rateLimitResetAt).
		Save(s.ctx)
	s.Require().NoError(err)
	name := "patched-name"
	s.Require().NoError(s.repo.UpdateWithExtraPatch(s.ctx, stale.ID, service.AccountColumnPatch{Name: &name}, map[string]any{
		"aether_ws": map[string]any{
			"enabled":                   true,
			"required_control_protocol": "route-v1",
		},
	}, []string{"legacy"}, stale.GroupIDs))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("patched-name", got.Name)
	s.Require().Equal("new-token", got.Credentials["runtime_token"])
	s.Require().Equal(service.StatusError, got.Status)
	s.Require().False(got.Schedulable)
	s.Require().NotNil(got.LastUsedAt)
	s.Require().Equal(lastUsedAt, got.LastUsedAt.UTC())
	s.Require().NotNil(got.RateLimitResetAt)
	s.Require().Equal(rateLimitResetAt, got.RateLimitResetAt.UTC())
	s.Require().Equal("2026-07-15T00:00:00Z", got.Extra["passive_usage_sampled_at"])
	s.Require().NotContains(got.Extra, "legacy")
	aetherWS, ok := got.Extra["aether_ws"].(map[string]any)
	s.Require().True(ok)
	s.Require().Equal("preserve", aetherWS["future_field"])
	s.Require().Equal(true, aetherWS["enabled"])
	s.Require().Equal("route-v1", aetherWS["required_control_protocol"])
}

func (s *AccountRepoSuite) TestUpdateWithExtraPatchDeleteThenSetReplacesAetherWS() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "replace-aether-ws-extra",
		Extra: map[string]any{
			"aether_ws": map[string]any{
				"enabled":      false,
				"future_field": "remove-me",
			},
		},
	})

	s.Require().NoError(s.repo.UpdateWithExtraPatch(
		s.ctx,
		account.ID,
		service.AccountColumnPatch{},
		map[string]any{"aether_ws": map[string]any{"enabled": true}},
		[]string{"aether_ws"},
		nil,
	))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(map[string]any{"enabled": true}, got.Extra["aether_ws"])
}

func (s *AccountRepoSuite) TestUpdateExtra_SchedulerNeutralSkipsOutboxAndSyncsFreshSnapshot() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "acc-extra-neutral",
		Platform: service.PlatformOpenAI,
		Extra:    map[string]any{"codex_usage_updated_at": "old"},
	})
	cacheRecorder := &schedulerCacheRecorder{
		accounts: map[int64]*service.Account{
			account.ID: {
				ID:       account.ID,
				Platform: account.Platform,
				Status:   service.StatusDisabled,
				Extra: map[string]any{
					"codex_usage_updated_at": "old",
				},
			},
		},
	}
	s.repo.schedulerCache = cacheRecorder

	updates := map[string]any{
		"codex_usage_updated_at":     "2026-03-11T10:00:00Z",
		"codex_5h_used_percent":      88.5,
		"session_window_utilization": 0.42,
	}
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, updates))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("2026-03-11T10:00:00Z", got.Extra["codex_usage_updated_at"])
	s.Require().Equal(88.5, got.Extra["codex_5h_used_percent"])
	s.Require().Equal(0.42, got.Extra["session_window_utilization"])

	var outboxCount int
	s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM scheduler_outbox", nil, &outboxCount))
	s.Require().Zero(outboxCount)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().NotNil(cacheRecorder.accounts[account.ID])
	s.Require().Equal(service.StatusActive, cacheRecorder.accounts[account.ID].Status)
	s.Require().Equal("2026-03-11T10:00:00Z", cacheRecorder.accounts[account.ID].Extra["codex_usage_updated_at"])
}

func (s *AccountRepoSuite) TestUpdateExtra_ExhaustedCodexSnapshotSyncsSchedulerCache() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "acc-extra-codex-exhausted",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Extra:    map[string]any{},
	})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder
	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{
		"codex_7d_used_percent":        100.0,
		"codex_7d_reset_at":            "2026-03-12T13:00:00Z",
		"codex_7d_reset_after_seconds": 86400,
	}))

	var count int
	err = scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM scheduler_outbox", nil, &count)
	s.Require().NoError(err)
	s.Require().Equal(0, count)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Equal(service.StatusActive, cacheRecorder.setAccounts[0].Status)
	s.Require().Equal(100.0, cacheRecorder.setAccounts[0].Extra["codex_7d_used_percent"])
}

func (s *AccountRepoSuite) TestUpdateExtra_SchedulerRelevantStillEnqueuesOutbox() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "acc-extra-mixed",
		Platform: service.PlatformAntigravity,
		Extra:    map[string]any{},
	})
	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{
		"mixed_scheduling":       true,
		"codex_usage_updated_at": "2026-03-11T10:00:00Z",
	}))

	var count int
	err = scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM scheduler_outbox", nil, &count)
	s.Require().NoError(err)
	s.Require().Equal(1, count)
}

// --- GetByCRSAccountID ---

func (s *AccountRepoSuite) TestGetByCRSAccountID() {
	crsID := "crs-12345"
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name:  "acc-crs",
		Extra: map[string]any{"crs_account_id": crsID},
	})

	got, err := s.repo.GetByCRSAccountID(s.ctx, crsID)
	s.Require().NoError(err)
	s.Require().NotNil(got)
	s.Require().Equal("acc-crs", got.Name)
}

func (s *AccountRepoSuite) TestGetByCRSAccountID_NotFound() {
	got, err := s.repo.GetByCRSAccountID(s.ctx, "non-existent")
	s.Require().NoError(err)
	s.Require().Nil(got)
}

func (s *AccountRepoSuite) TestGetByCRSAccountID_EmptyString() {
	got, err := s.repo.GetByCRSAccountID(s.ctx, "")
	s.Require().NoError(err)
	s.Require().Nil(got)
}

// --- BulkUpdate ---

func (s *AccountRepoSuite) TestBulkUpdate() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk1", Priority: 1})
	a2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk2", Priority: 1})

	newPriority := 99
	affected, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID, a2.ID}, service.AccountBulkUpdate{
		Priority: &newPriority,
	})
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(affected, int64(1), "expected at least one affected row")

	got1, _ := s.repo.GetByID(s.ctx, a1.ID)
	got2, _ := s.repo.GetByID(s.ctx, a2.ID)
	s.Require().Equal(99, got1.Priority)
	s.Require().Equal(99, got2.Priority)
}

func (s *AccountRepoSuite) TestBulkUpdateReturningIDsExcludesMissingAccounts() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-returning", Priority: 1})
	priority := 77

	updatedIDs, err := s.repo.BulkUpdateReturningIDs(s.ctx, []int64{a1.ID, 999999999}, service.AccountBulkUpdate{
		Priority: &priority,
	})

	s.Require().NoError(err)
	s.Require().Equal([]int64{a1.ID}, updatedIDs)
}

func (s *AccountRepoSuite) TestBulkUpdate_MergeCredentials() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "bulk-cred",
		Credentials: map[string]any{"existing": "value"},
	})

	_, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID}, service.AccountBulkUpdate{
		Credentials: map[string]any{"new_key": "new_value"},
	})
	s.Require().NoError(err)

	got, _ := s.repo.GetByID(s.ctx, a1.ID)
	s.Require().Equal("value", got.Credentials["existing"])
	s.Require().Equal("new_value", got.Credentials["new_key"])
}

func (s *AccountRepoSuite) TestBulkUpdate_MergeExtra() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:  "bulk-extra",
		Extra: map[string]any{"existing": "val"},
	})

	_, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID}, service.AccountBulkUpdate{
		Extra: map[string]any{"new_key": "new_val"},
	})
	s.Require().NoError(err)

	got, _ := s.repo.GetByID(s.ctx, a1.ID)
	s.Require().Equal("val", got.Extra["existing"])
	s.Require().Equal("new_val", got.Extra["new_key"])
}

func (s *AccountRepoSuite) TestBulkUpdate_DeepMergesAetherWSExtra() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "bulk-aether-ws-extra",
		Extra: map[string]any{
			"existing": "val",
			"aether_ws": map[string]any{
				"schema_version":            1,
				"enabled":                   true,
				"required_control_protocol": "route-v1",
				"future_field":              "preserve-me",
			},
		},
	})

	_, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID}, service.AccountBulkUpdate{
		Extra: map[string]any{
			"new_top_level": "new-value",
			"aether_ws": map[string]any{
				"enabled":          false,
				"new_future_field": true,
			},
		},
	})
	s.Require().NoError(err)

	got, err := s.repo.GetByID(s.ctx, a1.ID)
	s.Require().NoError(err)
	s.Require().Equal("val", got.Extra["existing"])
	s.Require().Equal("new-value", got.Extra["new_top_level"])
	aetherWS, ok := got.Extra["aether_ws"].(map[string]any)
	s.Require().True(ok)
	s.Require().Equal("preserve-me", aetherWS["future_field"])
	s.Require().Equal("route-v1", aetherWS["required_control_protocol"])
	s.Require().Equal(false, aetherWS["enabled"])
	s.Require().Equal(true, aetherWS["new_future_field"])
}

func (s *AccountRepoSuite) TestBulkUpdate_RepairsMalformedAetherWSExtra() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "bulk-malformed-aether-ws-extra",
		Extra: map[string]any{
			"existing":  "val",
			"aether_ws": "legacy-invalid-value",
		},
	})

	_, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID}, service.AccountBulkUpdate{
		Extra: map[string]any{
			"aether_ws": map[string]any{
				"schema_version":            1,
				"enabled":                   true,
				"required_control_protocol": "route-v1",
			},
		},
	})
	s.Require().NoError(err)

	got, err := s.repo.GetByID(s.ctx, a1.ID)
	s.Require().NoError(err)
	s.Require().Equal("val", got.Extra["existing"])
	aetherWS, ok := got.Extra["aether_ws"].(map[string]any)
	s.Require().True(ok)
	s.Require().Equal(true, aetherWS["enabled"])
	s.Require().Equal("route-v1", aetherWS["required_control_protocol"])
}

func (s *AccountRepoSuite) TestBulkUpdate_RejectsNonObjectAetherWSInput() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-invalid-aether-ws"})

	_, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID}, service.AccountBulkUpdate{
		Extra: map[string]any{"aether_ws": "invalid"},
	})

	s.Require().Error(err)
	got, getErr := s.repo.GetByID(s.ctx, a1.ID)
	s.Require().NoError(getErr)
	s.Require().NotContains(got.Extra, "aether_ws")
}

func (s *AccountRepoSuite) TestBulkUpdate_EmptyIDs() {
	affected, err := s.repo.BulkUpdate(s.ctx, []int64{}, service.AccountBulkUpdate{})
	s.Require().NoError(err)
	s.Require().Zero(affected)
}

func (s *AccountRepoSuite) TestBulkUpdate_EmptyUpdates() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-empty"})

	affected, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID}, service.AccountBulkUpdate{})
	s.Require().NoError(err)
	s.Require().Zero(affected)
}

func idsOfAccounts(accounts []service.Account) []int64 {
	out := make([]int64, 0, len(accounts))
	for i := range accounts {
		out = append(out, accounts[i].ID)
	}
	return out
}
