//go:build unit

package service

import (
	"context"
	"database/sql"
	"testing"

	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type txAwareCreateAccountRepo struct {
	mockAccountRepoForGemini
	createTx *dbent.Tx
	bindTx   *dbent.Tx
}

func (r *txAwareCreateAccountRepo) Create(ctx context.Context, account *Account) error {
	r.createTx = dbent.TxFromContext(ctx)
	account.ID = 901
	return nil
}

func (r *txAwareCreateAccountRepo) BindGroups(ctx context.Context, _ int64, _ []int64) error {
	r.bindTx = dbent.TxFromContext(ctx)
	return nil
}

type txAwareUpdateAccountRepo struct {
	atomicAccountPatchRepoStub
	patchTx *dbent.Tx
	bindTx  *dbent.Tx
}

func (r *txAwareUpdateAccountRepo) UpdateWithExtraPatch(
	ctx context.Context,
	id int64,
	columns AccountColumnPatch,
	set map[string]any,
	deleteKeys []string,
	groupIDs []int64,
) error {
	r.patchTx = dbent.TxFromContext(ctx)
	return r.atomicAccountPatchRepoStub.UpdateWithExtraPatch(ctx, id, columns, set, deleteKeys, groupIDs)
}

func (r *txAwareUpdateAccountRepo) BindGroups(ctx context.Context, _ int64, _ []int64) error {
	r.bindTx = dbent.TxFromContext(ctx)
	return nil
}

type txAwareBulkAccountRepo struct {
	accountRepoStubForBulkUpdate
	updateTx *dbent.Tx
	bindTx   *dbent.Tx
}

func (r *txAwareBulkAccountRepo) BulkUpdateReturningIDs(ctx context.Context, ids []int64, updates AccountBulkUpdate) ([]int64, error) {
	r.updateTx = dbent.TxFromContext(ctx)
	return r.accountRepoStubForBulkUpdate.BulkUpdateReturningIDs(ctx, ids, updates)
}

func (r *txAwareBulkAccountRepo) BulkBindGroups(ctx context.Context, accountIDs, _ []int64) error {
	r.bindTx = dbent.TxFromContext(ctx)
	r.bindGroupsCalls = append(r.bindGroupsCalls, accountIDs...)
	return nil
}

func newAdminAccountMutationTestClient(t *testing.T) *dbent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	driver := entsql.OpenDB("sqlite3", db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() {
		_ = client.Close()
		_ = db.Close()
	})
	return client
}

func TestCreateAccountAndBindGroupsShareOuterTransaction(t *testing.T) {
	repo := &txAwareCreateAccountRepo{}
	groupIDs := []int64{11}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 11, Name: "g11", Status: StatusActive}},
		entClient:   newAdminAccountMutationTestClient(t),
	}

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                  "tx-create",
		Platform:              PlatformOpenAI,
		Type:                  AccountTypeAPIKey,
		GroupIDs:              groupIDs,
		SkipMixedChannelCheck: true,
	})

	require.NoError(t, err)
	require.Equal(t, int64(901), account.ID)
	require.NotNil(t, repo.createTx)
	require.Same(t, repo.createTx, repo.bindTx)
	require.Equal(t, groupIDs, account.GroupIDs)
}

func TestUpdateAccountPatchAndBindGroupsShareOuterTransaction(t *testing.T) {
	base := &Account{
		ID:          902,
		Name:        "before",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Extra:       map[string]any{},
	}
	repo := &txAwareUpdateAccountRepo{atomicAccountPatchRepoStub: atomicAccountPatchRepoStub{account: base}}
	groupIDs := []int64{12}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 12, Name: "g12", Status: StatusActive}},
		entClient:   newAdminAccountMutationTestClient(t),
	}

	_, err := svc.UpdateAccount(context.Background(), base.ID, &UpdateAccountInput{
		Name:                  "after",
		GroupIDs:              &groupIDs,
		SkipMixedChannelCheck: true,
	})

	require.NoError(t, err)
	require.NotNil(t, repo.patchTx)
	require.Same(t, repo.patchTx, repo.bindTx)
}

func TestBulkUpdateAndBindGroupsShareOuterTransaction(t *testing.T) {
	repo := &txAwareBulkAccountRepo{}
	groupIDs := []int64{13}
	schedulable := false
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 13, Name: "g13", Status: StatusActive}},
		entClient:   newAdminAccountMutationTestClient(t),
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs:            []int64{1, 2},
		GroupIDs:              &groupIDs,
		Schedulable:           &schedulable,
		SkipMixedChannelCheck: true,
	})

	require.NoError(t, err)
	require.Equal(t, 2, result.Success)
	require.NotNil(t, repo.updateTx)
	require.Same(t, repo.updateTx, repo.bindTx)
	require.Equal(t, []int64{1, 2}, repo.bindGroupsCalls)
}
