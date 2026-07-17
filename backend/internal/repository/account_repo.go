// Package repository 实现数据访问层（Repository Pattern）。
//
// 该包提供了与数据库交互的所有操作，包括 CRUD、复杂查询和批量操作。
// 采用 Repository 模式将数据访问逻辑与业务逻辑分离，便于测试和维护。
//
// 主要特性：
//   - 使用 Ent ORM 进行类型安全的数据库操作
//   - 对于复杂查询（如批量更新、聚合统计）使用原生 SQL
//   - 提供统一的错误翻译机制，将数据库错误转换为业务错误
//   - 支持软删除，所有查询自动过滤已删除记录
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbaccountgroup "github.com/Wei-Shaw/sub2api/ent/accountgroup"
	dbgroup "github.com/Wei-Shaw/sub2api/ent/group"
	dbpredicate "github.com/Wei-Shaw/sub2api/ent/predicate"
	dbproxy "github.com/Wei-Shaw/sub2api/ent/proxy"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
)

// accountRepository 实现 service.AccountRepository 接口。
// 提供 AI API 账户的完整数据访问功能。
//
// 设计说明：
//   - client: Ent 客户端，用于类型安全的 ORM 操作
//   - sql: 原生 SQL 执行器，用于复杂查询和批量操作
//   - schedulerCache: 调度器缓存，用于在账号状态变更时同步快照
type accountRepository struct {
	client *dbent.Client // Ent ORM 客户端
	sql    sqlExecutor   // 原生 SQL 执行接口
	// schedulerCache 用于在账号状态变更时主动同步快照到缓存，
	// 确保粘性会话能及时感知账号不可用状态。
	// Used to proactively sync account snapshot to cache when status changes,
	// ensuring sticky sessions can promptly detect unavailable accounts.
	schedulerCache service.SchedulerCache
	fenceMu        sync.Mutex
	fenceTokens    map[*dbent.Tx]map[int64]int64
	fenceHooks     map[*dbent.Tx]struct{}
}

const (
	schedulerAccountMutationBeginTimeout   = 2 * time.Second
	schedulerAccountMutationFenceTTL       = 2 * time.Minute
	schedulerAccountMutationPublishTimeout = 3 * time.Second
)

var errSchedulerMutationTxContextRequired = errors.New("scheduler account mutation requires an explicit transaction context")

var schedulerNeutralExtraKeyPrefixes = []string{
	"codex_primary_",
	"codex_secondary_",
	"codex_5h_",
	"codex_7d_",
	"passive_usage_",
}

var schedulerNeutralExtraKeys = map[string]struct{}{
	"codex_usage_updated_at":     {},
	"session_window_utilization": {},
}

// NewAccountRepository 创建账户仓储实例。
// 这是对外暴露的构造函数，返回接口类型以便于依赖注入。
func NewAccountRepository(client *dbent.Client, sqlDB *sql.DB, schedulerCache service.SchedulerCache) service.AccountRepository {
	return newAccountRepositoryWithSQL(client, sqlDB, schedulerCache)
}

// newAccountRepositoryWithSQL 是内部构造函数，支持依赖注入 SQL 执行器。
// 这种设计便于单元测试时注入 mock 对象。
func newAccountRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor, schedulerCache service.SchedulerCache) *accountRepository {
	return &accountRepository{client: client, sql: sqlq, schedulerCache: schedulerCache}
}

func (r *accountRepository) Create(ctx context.Context, account *service.Account) error {
	if account == nil {
		return service.ErrAccountNilInput
	}

	client := clientFromContext(ctx, r.client)
	builder := client.Account.Create().
		SetName(account.Name).
		SetNillableNotes(account.Notes).
		SetPlatform(account.Platform).
		SetType(account.Type).
		SetCredentials(normalizeJSONMap(account.Credentials)).
		SetExtra(normalizeJSONMap(account.Extra)).
		SetConcurrency(account.Concurrency).
		SetPriority(account.Priority).
		SetStatus(account.Status).
		SetErrorMessage(account.ErrorMessage).
		SetSchedulable(account.Schedulable).
		SetAutoPauseOnExpired(account.AutoPauseOnExpired)

	if account.RateMultiplier != nil {
		builder.SetRateMultiplier(*account.RateMultiplier)
	}
	if account.LoadFactor != nil {
		builder.SetLoadFactor(*account.LoadFactor)
	}

	if account.ProxyID != nil {
		builder.SetProxyID(*account.ProxyID)
	}
	if account.LastUsedAt != nil {
		builder.SetLastUsedAt(*account.LastUsedAt)
	}
	if account.ExpiresAt != nil {
		builder.SetExpiresAt(*account.ExpiresAt)
	}
	if account.RateLimitedAt != nil {
		builder.SetRateLimitedAt(*account.RateLimitedAt)
	}
	if account.RateLimitResetAt != nil {
		builder.SetRateLimitResetAt(*account.RateLimitResetAt)
	}
	if account.OverloadUntil != nil {
		builder.SetOverloadUntil(*account.OverloadUntil)
	}
	if account.SessionWindowStart != nil {
		builder.SetSessionWindowStart(*account.SessionWindowStart)
	}
	if account.SessionWindowEnd != nil {
		builder.SetSessionWindowEnd(*account.SessionWindowEnd)
	}
	if account.SessionWindowStatus != "" {
		builder.SetSessionWindowStatus(account.SessionWindowStatus)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}

	account.ID = created.ID
	account.CreatedAt = created.CreatedAt
	account.UpdatedAt = created.UpdatedAt
	if err := enqueueSchedulerOutbox(ctx, client, service.SchedulerOutboxEventAccountChanged, &account.ID, nil, buildSchedulerGroupPayload(account.GroupIDs)); err != nil {
		if dbent.TxFromContext(ctx) != nil {
			return fmt.Errorf("enqueue account create outbox: %w", err)
		}
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue account create failed: account=%d err=%v", account.ID, err)
	}
	return nil
}

func (r *accountRepository) GetByID(ctx context.Context, id int64) (*service.Account, error) {
	m, err := r.client.Account.Query().Where(dbaccount.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}

	accounts, err := r.accountsToService(ctx, []*dbent.Account{m})
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, service.ErrAccountNotFound
	}
	return &accounts[0], nil
}

func (r *accountRepository) GetByIDs(ctx context.Context, ids []int64) ([]*service.Account, error) {
	if len(ids) == 0 {
		return []*service.Account{}, nil
	}

	// De-duplicate while preserving order of first occurrence.
	uniqueIDs := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return []*service.Account{}, nil
	}

	entAccounts, err := r.client.Account.
		Query().
		Where(dbaccount.IDIn(uniqueIDs...)).
		WithProxy().
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(entAccounts) == 0 {
		return []*service.Account{}, nil
	}

	accountIDs := make([]int64, 0, len(entAccounts))
	entByID := make(map[int64]*dbent.Account, len(entAccounts))
	for _, acc := range entAccounts {
		entByID[acc.ID] = acc
		accountIDs = append(accountIDs, acc.ID)
	}

	groupsByAccount, groupIDsByAccount, accountGroupsByAccount, err := r.loadAccountGroups(ctx, accountIDs)
	if err != nil {
		return nil, err
	}

	outByID := make(map[int64]*service.Account, len(entAccounts))
	for _, entAcc := range entAccounts {
		out := accountEntityToService(entAcc)
		if out == nil {
			continue
		}

		// Prefer the preloaded proxy edge when available.
		if entAcc.Edges.Proxy != nil {
			out.Proxy = proxyEntityToService(entAcc.Edges.Proxy)
		}

		if groups, ok := groupsByAccount[entAcc.ID]; ok {
			out.Groups = groups
		}
		if groupIDs, ok := groupIDsByAccount[entAcc.ID]; ok {
			out.GroupIDs = groupIDs
		}
		if ags, ok := accountGroupsByAccount[entAcc.ID]; ok {
			out.AccountGroups = ags
		}
		outByID[entAcc.ID] = out
	}

	// Preserve input order (first occurrence), and ignore missing IDs.
	out := make([]*service.Account, 0, len(uniqueIDs))
	for _, id := range uniqueIDs {
		if _, ok := entByID[id]; !ok {
			continue
		}
		if acc, ok := outByID[id]; ok && acc != nil {
			out = append(out, acc)
		}
	}

	return out, nil
}

// ExistsByID 检查指定 ID 的账号是否存在。
// 相比 GetByID，此方法性能更优，因为：
//   - 使用 Exist() 方法生成 SELECT EXISTS 查询，只返回布尔值
//   - 不加载完整的账号实体及其关联数据（Groups、Proxy 等）
//   - 适用于删除前的存在性检查等只需判断有无的场景
func (r *accountRepository) ExistsByID(ctx context.Context, id int64) (bool, error) {
	exists, err := r.client.Account.Query().Where(dbaccount.IDEQ(id)).Exist(ctx)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *accountRepository) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*service.Account, error) {
	if crsAccountID == "" {
		return nil, nil
	}

	// 使用 sqljson.ValueEQ 生成 JSON 路径过滤，避免手写 SQL 片段导致语法兼容问题。
	m, err := r.client.Account.Query().
		Where(func(s *entsql.Selector) {
			s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, crsAccountID, sqljson.Path("crs_account_id")))
		}).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	accounts, err := r.accountsToService(ctx, []*dbent.Account{m})
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, nil
	}
	return &accounts[0], nil
}

func (r *accountRepository) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, extra->>'crs_account_id'
		FROM accounts
		WHERE deleted_at IS NULL
			AND extra->>'crs_account_id' IS NOT NULL
			AND extra->>'crs_account_id' != ''
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]int64)
	for rows.Next() {
		var id int64
		var crsID string
		if err := rows.Scan(&id, &crsID); err != nil {
			return nil, err
		}
		result[crsID] = id
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *accountRepository) Update(ctx context.Context, account *service.Account) error {
	if account == nil {
		return nil
	}
	return r.withFencedAccountMutation(
		ctx,
		account.ID,
		service.SchedulerOutboxEventAccountChanged,
		buildSchedulerGroupPayload(account.GroupIDs),
		func(client *dbent.Client) error {
			return r.saveAccount(ctx, client, account, true)
		},
	)
}

// UpdateWithExtraPatch updates account columns and patches Extra in one
// transaction. It intentionally never writes the caller's full Extra snapshot.
func (r *accountRepository) UpdateWithExtraPatch(ctx context.Context, id int64, columns service.AccountColumnPatch, set map[string]any, deleteKeys []string, groupIDs []int64) error {
	if err := service.ValidateAccountAetherWSExtra(set); err != nil {
		return err
	}
	if !accountColumnPatchHasChanges(columns) && len(set) == 0 && len(deleteKeys) == 0 {
		return nil
	}

	var tx *dbent.Tx
	var txClient *dbent.Client
	ownsTx := false
	if ambientTx := dbent.TxFromContext(ctx); ambientTx != nil {
		txClient = ambientTx.Client()
	} else {
		var err error
		tx, err = r.client.Tx(ctx)
		if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
			return err
		}
		if err == nil {
			defer func() { _ = tx.Rollback() }()
			txClient = tx.Client()
			ownsTx = true
		} else {
			// The repository itself may already have been constructed with tx.Client().
			if r.schedulerMutationFenceEnabled() {
				return errSchedulerMutationTxContextRequired
			}
			txClient = r.client
		}
	}
	tokens, err := r.beginSchedulerAccountMutationsForTx(ctx, txOrAmbient(ctx, tx), []int64{id})
	if err != nil {
		return err
	}
	r.registerSchedulerAccountMutationHooks(txOrAmbient(ctx, tx), tokens)
	resolved := false
	if ownsTx {
		defer func() {
			if !resolved {
				r.rollbackSchedulerAccountMutation(tx, tokens, "account_patch_failed")
			}
		}()
	}

	columnUpdated, err := updateAccountColumns(ctx, txClient, id, columns)
	if err != nil {
		return err
	}
	extraUpdated := false
	if len(set) > 0 || len(deleteKeys) > 0 {
		if err := patchAccountExtra(ctx, txClient, id, set, deleteKeys); err != nil {
			return err
		}
		extraUpdated = true
	}
	if !columnUpdated && !extraUpdated {
		if ownsTx {
			commitErr := r.commitSchedulerAccountMutation(tx, tokens)
			resolved = true
			if commitErr != nil {
				return commitErr
			}
		}
		if r.shouldLegacySyncSchedulerCache(ctx, ownsTx) {
			r.syncSchedulerAccountSnapshot(ctx, id)
		}
		return nil
	}
	payload := withSchedulerEpoch(buildSchedulerGroupPayload(groupIDs), tokens[id])
	if err := enqueueSchedulerOutbox(ctx, txClient, service.SchedulerOutboxEventAccountChanged, &id, nil, payload); err != nil {
		return fmt.Errorf("enqueue account update outbox: %w", err)
	}
	if ownsTx {
		commitErr := r.commitSchedulerAccountMutation(tx, tokens)
		resolved = true
		if commitErr != nil {
			return commitErr
		}
	}
	if r.shouldLegacySyncSchedulerCache(ctx, ownsTx) {
		r.syncSchedulerAccountSnapshot(ctx, id)
	}
	return nil
}

func txOrAmbient(ctx context.Context, owned *dbent.Tx) *dbent.Tx {
	if owned != nil {
		return owned
	}
	return dbent.TxFromContext(ctx)
}

func (r *accountRepository) beginSchedulerAccountMutations(ctx context.Context, accountIDs []int64) (map[int64]int64, error) {
	mutationCache, ok := r.schedulerCache.(service.SchedulerAccountMutationCache)
	if !ok || mutationCache == nil {
		return nil, nil
	}
	beginCtx, cancel := context.WithTimeout(ctx, schedulerAccountMutationBeginTimeout)
	defer cancel()
	tokens, err := mutationCache.BeginAccountMutations(beginCtx, accountIDs, schedulerAccountMutationFenceTTL)
	if err != nil {
		// A pipelined bulk Begin can fence a subset before discovering a
		// conflicting account. Restore that subset from the unchanged database.
		if len(tokens) > 0 {
			r.publishSchedulerAccountMutations(tokens, "begin_failed")
		}
		return nil, fmt.Errorf("begin scheduler account mutation: %w", err)
	}
	return tokens, nil
}

func (r *accountRepository) beginSchedulerAccountMutationsForTx(ctx context.Context, tx *dbent.Tx, accountIDs []int64) (map[int64]int64, error) {
	if !r.schedulerMutationFenceEnabled() {
		return nil, nil
	}
	if tx == nil {
		return nil, errSchedulerMutationTxContextRequired
	}
	ids := uniquePositiveInt64s(accountIDs)
	if len(ids) == 0 {
		return nil, nil
	}

	// Ent transactions are used sequentially by these admin paths. Holding the
	// repository mutex across the short (2s bounded) Redis Begin also prevents
	// two goroutines from allocating different tokens for the same tx/account.
	r.fenceMu.Lock()
	defer r.fenceMu.Unlock()
	if r.fenceTokens == nil {
		r.fenceTokens = make(map[*dbent.Tx]map[int64]int64)
	}
	txTokens := r.fenceTokens[tx]
	if txTokens == nil {
		txTokens = make(map[int64]int64)
		r.fenceTokens[tx] = txTokens
	}
	missing := make([]int64, 0, len(ids))
	result := make(map[int64]int64, len(ids))
	for _, accountID := range ids {
		if epoch := txTokens[accountID]; epoch > 0 {
			result[accountID] = epoch
		} else {
			missing = append(missing, accountID)
		}
	}
	if len(missing) > 0 {
		newTokens, err := r.beginSchedulerAccountMutations(ctx, missing)
		if err != nil {
			if len(txTokens) == 0 {
				delete(r.fenceTokens, tx)
			}
			return nil, err
		}
		for accountID, epoch := range newTokens {
			txTokens[accountID] = epoch
			result[accountID] = epoch
		}
	}
	return result, nil
}

func (r *accountRepository) schedulerAccountMutationTokensForTx(tx *dbent.Tx) map[int64]int64 {
	if tx == nil {
		return nil
	}
	r.fenceMu.Lock()
	defer r.fenceMu.Unlock()
	return cloneSchedulerMutationTokens(r.fenceTokens[tx])
}

func (r *accountRepository) clearSchedulerAccountMutationTokens(tx *dbent.Tx) {
	if tx == nil {
		return
	}
	r.fenceMu.Lock()
	delete(r.fenceTokens, tx)
	delete(r.fenceHooks, tx)
	r.fenceMu.Unlock()
}

func (r *accountRepository) schedulerMutationFenceEnabled() bool {
	_, ok := r.schedulerCache.(service.SchedulerAccountMutationCache)
	return ok
}

func (r *accountRepository) shouldLegacySyncSchedulerCache(ctx context.Context, ownsTx bool) bool {
	return !r.schedulerMutationFenceEnabled() && (ownsTx || dbent.TxFromContext(ctx) == nil)
}

func (r *accountRepository) commitSchedulerAccountMutation(tx *dbent.Tx, tokens map[int64]int64) error {
	if tx == nil {
		return nil
	}
	if allTokens := r.schedulerAccountMutationTokensForTx(tx); len(allTokens) > 0 {
		tokens = allTokens
	}
	if err := tx.Commit(); err != nil {
		// Commit errors can be outcome-ambiguous. A successful rollback invokes
		// the registered rollback hook; otherwise reconcile synchronously from
		// the authoritative database using a detached, bounded context.
		r.rollbackSchedulerAccountMutation(tx, tokens, "commit_failed")
		return err
	}
	return nil
}

func (r *accountRepository) rollbackSchedulerAccountMutation(tx *dbent.Tx, tokens map[int64]int64, reason string) {
	if tx == nil {
		return
	}
	if allTokens := r.schedulerAccountMutationTokensForTx(tx); len(allTokens) > 0 {
		tokens = allTokens
	}
	if err := tx.Rollback(); err != nil {
		// database/sql may already have rolled the transaction back after context
		// cancellation. In that case no rollback hook runs for this call, so read
		// the now-authoritative DB state and release the fence explicitly.
		r.publishSchedulerAccountMutations(tokens, reason)
		r.clearSchedulerAccountMutationTokens(tx)
	}
}

func (r *accountRepository) registerSchedulerAccountMutationHooks(tx *dbent.Tx, tokens map[int64]int64) {
	if tx == nil || len(tokens) == 0 {
		return
	}
	r.fenceMu.Lock()
	if r.fenceHooks == nil {
		r.fenceHooks = make(map[*dbent.Tx]struct{})
	}
	if _, registered := r.fenceHooks[tx]; registered {
		r.fenceMu.Unlock()
		return
	}
	r.fenceHooks[tx] = struct{}{}
	r.fenceMu.Unlock()

	tx.OnCommit(func(next dbent.Committer) dbent.Committer {
		return dbent.CommitFunc(func(ctx context.Context, committedTx *dbent.Tx) error {
			if err := next.Commit(ctx, committedTx); err != nil {
				return err
			}
			r.publishSchedulerAccountMutations(r.schedulerAccountMutationTokensForTx(committedTx), "commit")
			r.clearSchedulerAccountMutationTokens(committedTx)
			return nil
		})
	})
	tx.OnRollback(func(next dbent.Rollbacker) dbent.Rollbacker {
		return dbent.RollbackFunc(func(ctx context.Context, rolledBackTx *dbent.Tx) error {
			if err := next.Rollback(ctx, rolledBackTx); err != nil {
				return err
			}
			r.publishSchedulerAccountMutations(r.schedulerAccountMutationTokensForTx(rolledBackTx), "rollback")
			r.clearSchedulerAccountMutationTokens(rolledBackTx)
			return nil
		})
	})
}

func cloneSchedulerMutationTokens(tokens map[int64]int64) map[int64]int64 {
	cloned := make(map[int64]int64, len(tokens))
	for accountID, epoch := range tokens {
		if accountID > 0 && epoch > 0 {
			cloned[accountID] = epoch
		}
	}
	return cloned
}

func (r *accountRepository) publishSchedulerAccountMutations(tokens map[int64]int64, reason string) {
	mutationCache, ok := r.schedulerCache.(service.SchedulerAccountMutationCache)
	if !ok || mutationCache == nil || len(tokens) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), schedulerAccountMutationPublishTimeout)
	defer cancel()
	ids := make([]int64, 0, len(tokens))
	for accountID := range tokens {
		ids = append(ids, accountID)
	}
	accounts, err := r.GetByIDs(ctx, ids)
	if err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerFence] refresh read failed: reason=%s count=%d err=%v", reason, len(ids), err)
		return
	}
	found := make(map[int64]*service.Account, len(accounts))
	for _, account := range accounts {
		if account != nil {
			found[account.ID] = account
		}
	}
	if batchCache, ok := r.schedulerCache.(service.SchedulerAccountMutationBatchCache); ok {
		results, reconcileErr := batchCache.ReconcileAccountMutations(ctx, found, tokens)
		if reconcileErr != nil {
			logger.LegacyPrintf("repository.account", "[SchedulerFence] batch publish failed: reason=%s count=%d err=%v", reason, len(tokens), reconcileErr)
			return
		}
		for accountID, epoch := range tokens {
			if !results[accountID] {
				logger.LegacyPrintf("repository.account", "[SchedulerFence] batch publish superseded: reason=%s account=%d epoch=%d", reason, accountID, epoch)
			}
		}
		return
	}
	for accountID, epoch := range tokens {
		if account := found[accountID]; account != nil {
			published, publishErr := mutationCache.PublishAccountMutation(ctx, account, epoch)
			if publishErr != nil {
				logger.LegacyPrintf("repository.account", "[SchedulerFence] publish failed: reason=%s account=%d epoch=%d err=%v", reason, accountID, epoch, publishErr)
			} else if !published {
				logger.LegacyPrintf("repository.account", "[SchedulerFence] publish superseded: reason=%s account=%d epoch=%d", reason, accountID, epoch)
			}
			continue
		}
		completed, completeErr := mutationCache.CompleteAccountDeletion(ctx, accountID, epoch)
		if completeErr != nil {
			logger.LegacyPrintf("repository.account", "[SchedulerFence] delete publish failed: reason=%s account=%d epoch=%d err=%v", reason, accountID, epoch, completeErr)
		} else if !completed {
			logger.LegacyPrintf("repository.account", "[SchedulerFence] delete publish superseded: reason=%s account=%d epoch=%d", reason, accountID, epoch)
		}
	}
}

func withSchedulerEpoch(payload map[string]any, epoch int64) map[string]any {
	if epoch <= 0 {
		return payload
	}
	result := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		result[key] = value
	}
	result["scheduler_epoch"] = strconv.FormatInt(epoch, 10)
	return result
}

func withSchedulerEpochs(payload map[string]any, tokens map[int64]int64) map[string]any {
	if len(tokens) == 0 {
		return payload
	}
	result := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		result[key] = value
	}
	epochs := make(map[string]any, len(tokens))
	for accountID, epoch := range tokens {
		if accountID > 0 && epoch > 0 {
			epochs[strconv.FormatInt(accountID, 10)] = strconv.FormatInt(epoch, 10)
		}
	}
	result["scheduler_epochs"] = epochs
	return result
}

func accountColumnPatchHasChanges(columns service.AccountColumnPatch) bool {
	return columns.Name != nil ||
		columns.NotesSet ||
		columns.Type != nil ||
		len(columns.Credentials) > 0 ||
		columns.ProxyIDSet ||
		columns.Concurrency != nil ||
		columns.Priority != nil ||
		columns.RateMultiplier != nil ||
		columns.LoadFactorSet ||
		columns.Status != nil ||
		columns.Schedulable != nil ||
		columns.ExpiresAtSet ||
		columns.AutoPauseOnExpired != nil
}

func (r *accountRepository) withFencedAccountMutation(
	ctx context.Context,
	accountID int64,
	eventType string,
	payload map[string]any,
	mutate func(*dbent.Client) error,
) error {
	if mutate == nil || accountID <= 0 {
		return nil
	}

	var tx *dbent.Tx
	var txClient *dbent.Client
	ownsTx := false
	if ambientTx := dbent.TxFromContext(ctx); ambientTx != nil {
		txClient = ambientTx.Client()
	} else {
		var err error
		tx, err = r.client.Tx(ctx)
		if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
			return err
		}
		if err == nil {
			defer func() { _ = tx.Rollback() }()
			txClient = tx.Client()
			ownsTx = true
		} else {
			if r.schedulerMutationFenceEnabled() {
				return errSchedulerMutationTxContextRequired
			}
			txClient = r.client
		}
	}

	tokens, err := r.beginSchedulerAccountMutationsForTx(ctx, txOrAmbient(ctx, tx), []int64{accountID})
	if err != nil {
		return err
	}
	r.registerSchedulerAccountMutationHooks(txOrAmbient(ctx, tx), tokens)
	resolved := false
	if ownsTx {
		defer func() {
			if !resolved {
				r.rollbackSchedulerAccountMutation(tx, tokens, "account_mutation_failed")
			}
		}()
	}
	if err := mutate(txClient); err != nil {
		return err
	}
	if eventType != "" {
		payload = withSchedulerEpoch(payload, tokens[accountID])
		if err := enqueueSchedulerOutbox(ctx, txClient, eventType, &accountID, nil, payload); err != nil {
			return fmt.Errorf("enqueue %s outbox: %w", eventType, err)
		}
	}
	if ownsTx {
		commitErr := r.commitSchedulerAccountMutation(tx, tokens)
		resolved = true
		if commitErr != nil {
			return commitErr
		}
	}
	if r.shouldLegacySyncSchedulerCache(ctx, ownsTx) {
		r.syncSchedulerAccountSnapshot(ctx, accountID)
	}
	return nil
}

func updateAccountColumns(ctx context.Context, client *dbent.Client, id int64, columns service.AccountColumnPatch) (bool, error) {
	builder := client.Account.UpdateOneID(id)
	changed := false
	if columns.Name != nil {
		builder.SetName(*columns.Name)
		changed = true
	}
	if columns.NotesSet {
		if columns.Notes == nil {
			builder.ClearNotes()
		} else {
			builder.SetNotes(*columns.Notes)
		}
		changed = true
	}
	if columns.Type != nil {
		builder.SetType(*columns.Type)
		changed = true
	}
	if len(columns.Credentials) > 0 {
		builder.SetCredentials(normalizeJSONMap(columns.Credentials))
		changed = true
	}
	if columns.ProxyIDSet {
		if columns.ProxyID == nil {
			builder.ClearProxyID()
		} else {
			builder.SetProxyID(*columns.ProxyID)
		}
		changed = true
	}
	if columns.Concurrency != nil {
		builder.SetConcurrency(*columns.Concurrency)
		changed = true
	}
	if columns.Priority != nil {
		builder.SetPriority(*columns.Priority)
		changed = true
	}
	if columns.RateMultiplier != nil {
		builder.SetRateMultiplier(*columns.RateMultiplier)
		changed = true
	}
	if columns.LoadFactorSet {
		if columns.LoadFactor == nil {
			builder.ClearLoadFactor()
		} else {
			builder.SetLoadFactor(*columns.LoadFactor)
		}
		changed = true
	}
	if columns.Status != nil {
		builder.SetStatus(*columns.Status)
		changed = true
	}
	if columns.Schedulable != nil {
		builder.SetSchedulable(*columns.Schedulable)
		changed = true
	}
	if columns.ExpiresAtSet {
		if columns.ExpiresAt == nil {
			builder.ClearExpiresAt()
		} else {
			builder.SetExpiresAt(*columns.ExpiresAt)
		}
		changed = true
	}
	if columns.AutoPauseOnExpired != nil {
		builder.SetAutoPauseOnExpired(*columns.AutoPauseOnExpired)
		changed = true
	}
	if !changed {
		return false, nil
	}
	if _, err := builder.Save(ctx); err != nil {
		return false, translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}
	return true, nil
}

func (r *accountRepository) saveAccount(ctx context.Context, client *dbent.Client, account *service.Account, includeExtra bool) error {
	if account == nil {
		return nil
	}
	schedulable := account.Schedulable
	if account.Status == service.StatusError {
		schedulable = false
	}

	builder := client.Account.UpdateOneID(account.ID).
		SetName(account.Name).
		SetNillableNotes(account.Notes).
		SetPlatform(account.Platform).
		SetType(account.Type).
		SetCredentials(normalizeJSONMap(account.Credentials)).
		SetConcurrency(account.Concurrency).
		SetPriority(account.Priority).
		SetStatus(account.Status).
		SetErrorMessage(account.ErrorMessage).
		SetSchedulable(schedulable).
		SetAutoPauseOnExpired(account.AutoPauseOnExpired)
	if includeExtra {
		builder.SetExtra(normalizeJSONMap(account.Extra))
	}

	if account.RateMultiplier != nil {
		builder.SetRateMultiplier(*account.RateMultiplier)
	}
	if account.LoadFactor != nil {
		builder.SetLoadFactor(*account.LoadFactor)
	} else {
		builder.ClearLoadFactor()
	}

	if account.ProxyID != nil {
		builder.SetProxyID(*account.ProxyID)
	} else {
		builder.ClearProxyID()
	}
	if account.LastUsedAt != nil {
		builder.SetLastUsedAt(*account.LastUsedAt)
	} else {
		builder.ClearLastUsedAt()
	}
	if account.ExpiresAt != nil {
		builder.SetExpiresAt(*account.ExpiresAt)
	} else {
		builder.ClearExpiresAt()
	}
	if account.RateLimitedAt != nil {
		builder.SetRateLimitedAt(*account.RateLimitedAt)
	} else {
		builder.ClearRateLimitedAt()
	}
	if account.RateLimitResetAt != nil {
		builder.SetRateLimitResetAt(*account.RateLimitResetAt)
	} else {
		builder.ClearRateLimitResetAt()
	}
	if account.OverloadUntil != nil {
		builder.SetOverloadUntil(*account.OverloadUntil)
	} else {
		builder.ClearOverloadUntil()
	}
	if account.SessionWindowStart != nil {
		builder.SetSessionWindowStart(*account.SessionWindowStart)
	} else {
		builder.ClearSessionWindowStart()
	}
	if account.SessionWindowEnd != nil {
		builder.SetSessionWindowEnd(*account.SessionWindowEnd)
	} else {
		builder.ClearSessionWindowEnd()
	}
	if account.SessionWindowStatus != "" {
		builder.SetSessionWindowStatus(account.SessionWindowStatus)
	} else {
		builder.ClearSessionWindowStatus()
	}
	if account.Notes == nil {
		builder.ClearNotes()
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}
	account.UpdatedAt = updated.UpdatedAt
	return nil
}

func patchAccountExtra(ctx context.Context, client *dbent.Client, id int64, set map[string]any, deleteKeys []string) error {
	if err := service.ValidateAccountAetherWSExtra(set); err != nil {
		return err
	}
	if set == nil {
		set = map[string]any{}
	}
	payload, err := json.Marshal(set)
	if err != nil {
		return err
	}

	uniqueDeleteKeys := make([]string, 0, len(deleteKeys))
	seenDeleteKeys := make(map[string]struct{}, len(deleteKeys))
	for _, key := range deleteKeys {
		if key == "" {
			continue
		}
		if _, exists := seenDeleteKeys[key]; exists {
			continue
		}
		seenDeleteKeys[key] = struct{}{}
		uniqueDeleteKeys = append(uniqueDeleteKeys, key)
	}

	baseExtra := "(CASE WHEN jsonb_typeof(extra) = 'object' THEN extra ELSE '{}'::jsonb END)"
	afterDelete := "(" + baseExtra + " - $1::text[])"
	incoming := "$2::jsonb"
	existingAetherWS := "(CASE WHEN jsonb_typeof(" + afterDelete + "->'aether_ws') = 'object' THEN " + afterDelete + "->'aether_ws' ELSE '{}'::jsonb END)"
	incomingAetherWS := "(CASE WHEN jsonb_typeof(" + incoming + "->'aether_ws') = 'object' THEN " + incoming + "->'aether_ws' ELSE '{}'::jsonb END)"
	patchedExtra := "CASE WHEN " + incoming + " ? 'aether_ws' THEN " +
		"jsonb_set(" + afterDelete + " || (" + incoming + " - 'aether_ws'), '{aether_ws}', " +
		existingAetherWS + " || " + incomingAetherWS + ", true) " +
		"ELSE " + afterDelete + " || " + incoming + " END"

	result, err := client.ExecContext(ctx,
		"UPDATE accounts SET extra = "+patchedExtra+", updated_at = NOW() WHERE id = $3 AND deleted_at IS NULL",
		pq.Array(uniqueDeleteKeys), string(payload), id,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	return nil
}

func (r *accountRepository) UpdateCredentials(ctx context.Context, id int64, credentials map[string]any) error {
	return r.withFencedAccountMutation(ctx, id, service.SchedulerOutboxEventAccountChanged, nil, func(client *dbent.Client) error {
		_, err := client.Account.UpdateOneID(id).
			SetCredentials(normalizeJSONMap(credentials)).
			Save(ctx)
		return translatePersistenceError(err, service.ErrAccountNotFound, nil)
	})
}

func (r *accountRepository) Delete(ctx context.Context, id int64) error {
	groupIDs, err := r.loadAccountGroupIDs(ctx, id)
	if err != nil {
		return err
	}
	var tx *dbent.Tx
	var txClient *dbent.Client
	ownsTx := false
	if ambientTx := dbent.TxFromContext(ctx); ambientTx != nil {
		txClient = ambientTx.Client()
	} else {
		tx, err = r.client.Tx(ctx)
		if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
			return err
		}
		if err == nil {
			defer func() { _ = tx.Rollback() }()
			txClient = tx.Client()
			ownsTx = true
		} else {
			if r.schedulerMutationFenceEnabled() {
				return errSchedulerMutationTxContextRequired
			}
			txClient = r.client
		}
	}
	tokens, err := r.beginSchedulerAccountMutationsForTx(ctx, txOrAmbient(ctx, tx), []int64{id})
	if err != nil {
		return err
	}
	r.registerSchedulerAccountMutationHooks(txOrAmbient(ctx, tx), tokens)
	resolved := false
	if ownsTx {
		defer func() {
			if !resolved {
				r.rollbackSchedulerAccountMutation(tx, tokens, "account_delete_failed")
			}
		}()
	}

	if _, err := txClient.AccountGroup.Delete().Where(dbaccountgroup.AccountIDEQ(id)).Exec(ctx); err != nil {
		return err
	}
	if _, err := txClient.ExecContext(ctx, "DELETE FROM scheduled_test_plans WHERE account_id = $1", id); err != nil {
		return err
	}
	if _, err := txClient.Account.Delete().Where(dbaccount.IDEQ(id)).Exec(ctx); err != nil {
		return err
	}
	payload := withSchedulerEpoch(buildSchedulerGroupPayload(groupIDs), tokens[id])
	if err := enqueueSchedulerOutbox(ctx, txClient, service.SchedulerOutboxEventAccountChanged, &id, nil, payload); err != nil {
		return fmt.Errorf("enqueue account delete outbox: %w", err)
	}
	if ownsTx {
		commitErr := r.commitSchedulerAccountMutation(tx, tokens)
		resolved = true
		if commitErr != nil {
			return commitErr
		}
	}
	if r.shouldLegacySyncSchedulerCache(ctx, ownsTx) {
		r.deleteSchedulerAccountSnapshot(ctx, id)
	}
	return nil
}

func (r *accountRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.Account, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "", "", 0, "")
}

func (r *accountRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]service.Account, *pagination.PaginationResult, error) {
	q := r.client.Account.Query()

	if platform != "" {
		q = q.Where(dbaccount.PlatformEQ(platform))
	}
	if accountType != "" {
		q = q.Where(dbaccount.TypeEQ(accountType))
	}
	if status != "" {
		switch status {
		case service.StatusActive:
			q = q.Where(
				activeOrPoolModeErrorStatusPredicate(),
				dbaccount.SchedulableEQ(true),
				nonPoolModeRateLimitAvailablePredicate(time.Now()),
				tempUnschedulablePredicate(),
			)
		case "rate_limited":
			q = q.Where(
				activeOrPoolModeErrorStatusPredicate(),
				nonPoolModeRateLimitedPredicate(time.Now()),
				tempUnschedulablePredicate(),
			)
		case "temp_unschedulable":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.And(
						entsql.Not(entsql.IsNull(col)),
						entsql.GT(col, entsql.Expr("NOW()")),
					))
				}),
			)
		case "unschedulable":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbaccount.SchedulableEQ(false),
				dbaccount.Or(
					dbaccount.RateLimitResetAtIsNil(),
					dbaccount.RateLimitResetAtLTE(time.Now()),
				),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.Or(
						entsql.IsNull(col),
						entsql.LTE(col, entsql.Expr("NOW()")),
					))
				}),
			)
		default:
			if status == service.StatusError {
				q = q.Where(nonPoolModeErrorStatusPredicate())
			} else {
				q = q.Where(dbaccount.StatusEQ(status))
			}
		}
	}
	if search != "" {
		q = q.Where(dbaccount.NameContainsFold(search))
	}
	if groupID == service.AccountListGroupUngrouped {
		q = q.Where(dbaccount.Not(dbaccount.HasAccountGroups()))
	} else if groupID > 0 {
		q = q.Where(dbaccount.HasAccountGroupsWith(dbaccountgroup.GroupIDEQ(groupID)))
	}
	if privacyMode != "" {
		q = q.Where(dbpredicate.Account(func(s *entsql.Selector) {
			path := sqljson.Path("privacy_mode")
			switch privacyMode {
			case service.AccountPrivacyModeUnsetFilter:
				s.Where(entsql.Or(
					entsql.Not(sqljson.HasKey(dbaccount.FieldExtra, path)),
					sqljson.ValueEQ(dbaccount.FieldExtra, "", path),
				))
			default:
				s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, privacyMode, path))
			}
		}))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	accountsQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range accountListOrder(params) {
		accountsQuery = accountsQuery.Order(order)
	}

	accounts, err := accountsQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}

	outAccounts, err := r.accountsToService(ctx, accounts)
	if err != nil {
		return nil, nil, err
	}
	return outAccounts, paginationResultFromTotal(int64(total), params), nil
}

func accountListOrder(params pagination.PaginationParams) []func(*entsql.Selector) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderAsc)

	field := dbaccount.FieldName
	defaultOrder := true
	switch sortBy {
	case "", "name":
		field = dbaccount.FieldName
	case "id":
		field = dbaccount.FieldID
		defaultOrder = false
	case "status":
		field = dbaccount.FieldStatus
		defaultOrder = false
	case "schedulable":
		field = dbaccount.FieldSchedulable
		defaultOrder = false
	case "priority":
		field = dbaccount.FieldPriority
		defaultOrder = false
	case "rate_multiplier":
		field = dbaccount.FieldRateMultiplier
		defaultOrder = false
	case "last_used_at":
		field = dbaccount.FieldLastUsedAt
		defaultOrder = false
	case "expires_at":
		field = dbaccount.FieldExpiresAt
		defaultOrder = false
	case "created_at":
		field = dbaccount.FieldCreatedAt
		defaultOrder = false
	}

	if sortOrder == pagination.SortOrderDesc {
		return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(dbaccount.FieldID)}
	}
	if defaultOrder {
		return []func(*entsql.Selector){dbent.Asc(dbaccount.FieldName), dbent.Asc(dbaccount.FieldID)}
	}
	return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(dbaccount.FieldID)}
}

func (r *accountRepository) ListByGroup(ctx context.Context, groupID int64) ([]service.Account, error) {
	accounts, err := r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status: service.StatusActive,
	})
	if err != nil {
		return nil, err
	}
	return accounts, nil
}

func (r *accountRepository) ListActive(ctx context.Context) ([]service.Account, error) {
	accounts, err := r.client.Account.Query().
		Where(activeOrPoolModeErrorStatusPredicate()).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformEQ(platform),
			activeOrPoolModeErrorStatusPredicate(),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) UpdateLastUsed(ctx context.Context, id int64) error {
	now := time.Now()
	_, err := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetLastUsedAt(now).
		Save(ctx)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"last_used": map[string]int64{
			strconv.FormatInt(id, 10): now.Unix(),
		},
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountLastUsed, &id, nil, payload); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue last used failed: account=%d err=%v", id, err)
	}
	return nil
}

func (r *accountRepository) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	if len(updates) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(updates))
	args := make([]any, 0, len(updates)*2+1)
	caseSQL := "UPDATE accounts SET last_used_at = CASE id"

	idx := 1
	for id, ts := range updates {
		caseSQL += " WHEN $" + itoa(idx) + " THEN $" + itoa(idx+1) + "::timestamptz"
		args = append(args, id, ts)
		ids = append(ids, id)
		idx += 2
	}

	caseSQL += " END, updated_at = NOW() WHERE id = ANY($" + itoa(idx) + ") AND deleted_at IS NULL"
	args = append(args, pq.Array(ids))

	_, err := r.sql.ExecContext(ctx, caseSQL, args...)
	if err != nil {
		return err
	}
	lastUsedPayload := make(map[string]int64, len(updates))
	for id, ts := range updates {
		lastUsedPayload[strconv.FormatInt(id, 10)] = ts.Unix()
	}
	payload := map[string]any{"last_used": lastUsedPayload}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountLastUsed, nil, nil, payload); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue batch last used failed: err=%v", err)
	}
	return nil
}

func (r *accountRepository) SetError(ctx context.Context, id int64, errorMsg string) error {
	return r.withFencedAccountMutation(ctx, id, service.SchedulerOutboxEventAccountChanged, nil, func(client *dbent.Client) error {
		_, err := client.Account.Update().
			Where(dbaccount.IDEQ(id)).
			SetStatus(service.StatusError).
			SetErrorMessage(errorMsg).
			SetSchedulable(false).
			Save(ctx)
		return err
	})
}

// syncSchedulerAccountSnapshot 在账号状态变更时主动同步快照到调度器缓存。
// 当账号被设置为错误、禁用、不可调度或临时不可调度时调用，
// 确保调度器和粘性会话逻辑能及时感知账号的最新状态，避免继续使用不可用账号。
//
// syncSchedulerAccountSnapshot proactively syncs account snapshot to scheduler cache
// when account status changes. Called when account is set to error, disabled,
// unschedulable, or temporarily unschedulable, ensuring scheduler and sticky session
// logic can promptly detect the latest account state and avoid using unavailable accounts.
func (r *accountRepository) syncSchedulerAccountSnapshot(ctx context.Context, accountID int64) {
	if r == nil || r.schedulerCache == nil || accountID <= 0 {
		return
	}
	account, err := r.GetByID(ctx, accountID)
	if err != nil {
		logger.LegacyPrintf("repository.account", "[Scheduler] sync account snapshot read failed: id=%d err=%v", accountID, err)
		return
	}
	if err := r.schedulerCache.SetAccount(ctx, account); err != nil {
		logger.LegacyPrintf("repository.account", "[Scheduler] sync account snapshot write failed: id=%d err=%v", accountID, err)
	}
}

func (r *accountRepository) deleteSchedulerAccountSnapshot(ctx context.Context, accountID int64) {
	if r == nil || r.schedulerCache == nil || accountID <= 0 {
		return
	}
	if err := r.schedulerCache.DeleteAccount(ctx, accountID); err != nil {
		logger.LegacyPrintf("repository.account", "[Scheduler] delete account snapshot failed: id=%d err=%v", accountID, err)
	}
}

func (r *accountRepository) syncSchedulerAccountSnapshots(ctx context.Context, accountIDs []int64) {
	if r == nil || r.schedulerCache == nil || len(accountIDs) == 0 {
		return
	}

	uniqueIDs := make([]int64, 0, len(accountIDs))
	seen := make(map[int64]struct{}, len(accountIDs))
	for _, id := range accountIDs {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return
	}

	accounts, err := r.GetByIDs(ctx, uniqueIDs)
	if err != nil {
		logger.LegacyPrintf("repository.account", "[Scheduler] batch sync account snapshot read failed: count=%d err=%v", len(uniqueIDs), err)
		return
	}

	for _, account := range accounts {
		if account == nil {
			continue
		}
		if err := r.schedulerCache.SetAccount(ctx, account); err != nil {
			logger.LegacyPrintf("repository.account", "[Scheduler] batch sync account snapshot write failed: id=%d err=%v", account.ID, err)
		}
	}
}

func (r *accountRepository) ClearError(ctx context.Context, id int64) error {
	return r.withFencedAccountMutation(ctx, id, service.SchedulerOutboxEventAccountChanged, nil, func(client *dbent.Client) error {
		_, err := client.Account.Update().
			Where(dbaccount.IDEQ(id)).
			SetStatus(service.StatusActive).
			SetErrorMessage("").
			Save(ctx)
		return err
	})
}

func (r *accountRepository) AddToGroup(ctx context.Context, accountID, groupID int64, priority int) error {
	bindings, err := expandAccountGroupBindingsWithCustomGroups(ctx, clientFromContext(ctx, r.client), []int64{groupID})
	if err != nil {
		return err
	}
	if len(bindings) == 0 {
		return nil
	}
	payload := buildSchedulerGroupPayload(accountGroupBindingGroupIDs(bindings))
	return r.withFencedAccountMutation(ctx, accountID, service.SchedulerOutboxEventAccountGroupsChanged, payload, func(client *dbent.Client) error {
		if err := lockAccountGroupsForMutation(ctx, client, accountGroupBindingGroupIDs(bindings)); err != nil {
			return err
		}
		builders := make([]*dbent.AccountGroupCreate, 0, len(bindings))
		for _, binding := range bindings {
			builders = append(builders, client.AccountGroup.Create().
				SetAccountID(accountID).
				SetGroupID(binding.GroupID).
				SetPriority(priority))
		}
		return client.AccountGroup.CreateBulk(builders...).
			OnConflictColumns(dbaccountgroup.FieldAccountID, dbaccountgroup.FieldGroupID).
			UpdateNewValues().
			Exec(ctx)
	})
}

func (r *accountRepository) RemoveFromGroup(ctx context.Context, accountID, groupID int64) error {
	groupIDs, expandErr := expandGroupIDsWithCustomGroups(ctx, clientFromContext(ctx, r.client), []int64{groupID})
	if expandErr != nil {
		return expandErr
	}
	if len(groupIDs) == 0 {
		return nil
	}
	payload := buildSchedulerGroupPayload(groupIDs)
	return r.withFencedAccountMutation(ctx, accountID, service.SchedulerOutboxEventAccountGroupsChanged, payload, func(client *dbent.Client) error {
		if err := lockAccountGroupsForMutation(ctx, client, groupIDs); err != nil {
			return err
		}
		_, err := client.AccountGroup.Delete().
			Where(
				dbaccountgroup.AccountIDEQ(accountID),
				dbaccountgroup.GroupIDIn(groupIDs...),
			).
			Exec(ctx)
		return err
	})
}

func (r *accountRepository) GetGroups(ctx context.Context, accountID int64) ([]service.Group, error) {
	groups, err := r.client.Group.Query().
		Where(
			dbgroup.HasAccountsWith(dbaccount.IDEQ(accountID)),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	outGroups := make([]service.Group, 0, len(groups))
	for i := range groups {
		outGroups = append(outGroups, *groupEntityToService(groups[i]))
	}
	return outGroups, nil
}

func (r *accountRepository) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	existingGroupIDs, err := r.loadAccountGroupIDs(ctx, accountID)
	if err != nil {
		return err
	}
	var tx *dbent.Tx
	var txClient *dbent.Client
	ownsTx := false
	if ambientTx := dbent.TxFromContext(ctx); ambientTx != nil {
		txClient = ambientTx.Client()
	} else {
		tx, err = r.client.Tx(ctx)
		if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
			return err
		}
		if err == nil {
			defer func() { _ = tx.Rollback() }()
			txClient = tx.Client()
			ownsTx = true
		} else {
			// The repository itself may already use tx.Client().
			if r.schedulerMutationFenceEnabled() {
				return errSchedulerMutationTxContextRequired
			}
			txClient = r.client
		}
	}

	bindings, err := expandAccountGroupBindingsWithCustomGroups(ctx, txClient, groupIDs)
	if err != nil {
		return err
	}
	expandedGroupIDs := accountGroupBindingGroupIDs(bindings)
	if err := lockAccountGroupsForMutation(ctx, txClient, mergeGroupIDs(existingGroupIDs, expandedGroupIDs)); err != nil {
		return err
	}
	tokens, err := r.beginSchedulerAccountMutationsForTx(ctx, txOrAmbient(ctx, tx), []int64{accountID})
	if err != nil {
		return err
	}
	r.registerSchedulerAccountMutationHooks(txOrAmbient(ctx, tx), tokens)
	// Re-read after both the group lock and account fence. A concurrent
	// group-wide replacement may have committed while the initial routing
	// metadata was being prepared; the final outbox must rebuild every group
	// this replacement actually removes.
	existingGroupIDs, err = loadAccountGroupIDsWithClient(ctx, txClient, accountID)
	if err != nil {
		return err
	}
	resolved := false
	if ownsTx {
		defer func() {
			if !resolved {
				r.rollbackSchedulerAccountMutation(tx, tokens, "account_groups_failed")
			}
		}()
	}

	if _, err := txClient.AccountGroup.Delete().Where(dbaccountgroup.AccountIDEQ(accountID)).Exec(ctx); err != nil {
		return err
	}

	if len(bindings) > 0 {
		builders := make([]*dbent.AccountGroupCreate, 0, len(bindings))
		for _, binding := range bindings {
			builders = append(builders, txClient.AccountGroup.Create().
				SetAccountID(accountID).
				SetGroupID(binding.GroupID).
				SetPriority(binding.Priority),
			)
		}

		if _, err := txClient.AccountGroup.CreateBulk(builders...).Save(ctx); err != nil {
			return err
		}
	}

	payload := withSchedulerEpoch(
		buildSchedulerGroupPayload(mergeGroupIDs(existingGroupIDs, expandedGroupIDs)),
		tokens[accountID],
	)
	if err := enqueueSchedulerOutbox(ctx, txClient, service.SchedulerOutboxEventAccountGroupsChanged, &accountID, nil, payload); err != nil {
		return fmt.Errorf("enqueue bind groups outbox: %w", err)
	}
	if ownsTx {
		commitErr := r.commitSchedulerAccountMutation(tx, tokens)
		resolved = true
		if commitErr != nil {
			return commitErr
		}
	}
	if r.shouldLegacySyncSchedulerCache(ctx, ownsTx) {
		r.syncSchedulerAccountSnapshot(ctx, accountID)
	}
	return nil
}

// BulkBindGroups replaces the group bindings for all accounts in one fenced
// transaction. It is used by admin bulk edits so account columns and routing
// membership can share the caller's outer transaction without an N+1 write.
func (r *accountRepository) BulkBindGroups(ctx context.Context, accountIDs, groupIDs []int64) error {
	accountIDs = uniquePositiveInt64s(accountIDs)
	if len(accountIDs) == 0 {
		return nil
	}
	_, err := r.withFencedAccountGroupBatch(ctx, accountIDs, nil, nil, func(exec sqlExecutor, fencedIDs []int64) (int64, []int64, error) {
		bindings, err := expandAccountGroupBindingsWithCustomGroups(ctx, clientFromContext(ctx, r.client), groupIDs)
		if err != nil {
			return 0, nil, err
		}
		existingGroupIDs, err := queryDistinctAccountGroupIDs(ctx, exec, fencedIDs)
		if err != nil {
			return 0, nil, err
		}
		if err := lockAccountGroupsForMutation(ctx, exec, mergeGroupIDs(existingGroupIDs, accountGroupBindingGroupIDs(bindings))); err != nil {
			return 0, nil, err
		}
		deleted, err := exec.ExecContext(ctx, "DELETE FROM account_groups WHERE account_id = ANY($1)", pq.Array(fencedIDs))
		if err != nil {
			return 0, nil, err
		}
		if len(bindings) > 0 {
			bindingGroupIDs := make([]int64, 0, len(bindings))
			priorities := make([]int, 0, len(bindings))
			for _, binding := range bindings {
				bindingGroupIDs = append(bindingGroupIDs, binding.GroupID)
				priorities = append(priorities, binding.Priority)
			}
			if _, err := exec.ExecContext(ctx, `
				INSERT INTO account_groups (account_id, group_id, priority, created_at)
				SELECT accounts.account_id, bindings.group_id, bindings.priority, NOW()
				FROM unnest($1::bigint[]) AS accounts(account_id)
				CROSS JOIN unnest($2::bigint[], $3::integer[]) AS bindings(group_id, priority)
			`, pq.Array(fencedIDs), pq.Array(bindingGroupIDs), pq.Array(priorities)); err != nil {
				return 0, nil, err
			}
		}
		affected, _ := deleted.RowsAffected()
		return affected, mergeGroupIDs(existingGroupIDs, accountGroupBindingGroupIDs(bindings)), nil
	})
	return err
}

// BulkAddAccountsToGroup preserves existing memberships while adding one group
// to many accounts under the account mutation fence.
func (r *accountRepository) BulkAddAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	accountIDs = uniquePositiveInt64s(accountIDs)
	if groupID <= 0 || len(accountIDs) == 0 {
		return nil
	}
	_, err := r.withFencedAccountGroupBatch(ctx, accountIDs, []int64{groupID}, nil, func(exec sqlExecutor, fencedIDs []int64) (int64, []int64, error) {
		result, err := exec.ExecContext(ctx, `
			INSERT INTO account_groups (account_id, group_id, priority, created_at)
			SELECT account_id, $2, 50, NOW()
			FROM unnest($1::bigint[]) AS accounts(account_id)
			ON CONFLICT (account_id, group_id) DO NOTHING
		`, pq.Array(fencedIDs), groupID)
		if err != nil {
			return 0, nil, err
		}
		affected, _ := result.RowsAffected()
		return affected, []int64{groupID}, nil
	})
	return err
}

// RemoveAllAccountsFromGroups removes every membership for the supplied groups
// and fences exactly the accounts observed in the same database transaction.
func (r *accountRepository) RemoveAllAccountsFromGroups(ctx context.Context, groupIDs []int64) (int64, error) {
	groupIDs = uniquePositiveInt64s(groupIDs)
	if len(groupIDs) == 0 {
		return 0, nil
	}
	return r.withFencedAccountGroupBatch(ctx, nil, groupIDs, func(exec sqlExecutor) ([]int64, error) {
		return queryDistinctGroupAccountIDs(ctx, exec, groupIDs)
	}, func(exec sqlExecutor, _ []int64) (int64, []int64, error) {
		result, err := exec.ExecContext(ctx, "DELETE FROM account_groups WHERE group_id = ANY($1)", pq.Array(groupIDs))
		if err != nil {
			return 0, nil, err
		}
		affected, _ := result.RowsAffected()
		return affected, groupIDs, nil
	})
}

// ReplaceAccountsForGroup atomically replaces one group's account membership.
// Other group memberships on those accounts are left untouched.
func (r *accountRepository) ReplaceAccountsForGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	accountIDs = uniquePositiveInt64s(accountIDs)
	if groupID <= 0 {
		return nil
	}
	_, err := r.withFencedAccountGroupBatch(ctx, nil, []int64{groupID}, func(exec sqlExecutor) ([]int64, error) {
		existingIDs, err := queryDistinctGroupAccountIDs(ctx, exec, []int64{groupID})
		if err != nil {
			return nil, err
		}
		return mergeGroupIDs(existingIDs, accountIDs), nil
	}, func(exec sqlExecutor, _ []int64) (int64, []int64, error) {
		result, err := exec.ExecContext(ctx, "DELETE FROM account_groups WHERE group_id = $1", groupID)
		if err != nil {
			return 0, nil, err
		}
		affected, _ := result.RowsAffected()
		if len(accountIDs) > 0 {
			inserted, insertErr := exec.ExecContext(ctx, `
				INSERT INTO account_groups (account_id, group_id, priority, created_at)
				SELECT account_id, $2, 50, NOW()
				FROM unnest($1::bigint[]) AS accounts(account_id)
				ON CONFLICT (account_id, group_id) DO NOTHING
			`, pq.Array(accountIDs), groupID)
			if insertErr != nil {
				return 0, nil, insertErr
			}
			insertedRows, _ := inserted.RowsAffected()
			affected += insertedRows
		}
		return affected, []int64{groupID}, nil
	})
	return err
}

type accountGroupBatchResolver func(exec sqlExecutor) ([]int64, error)
type accountGroupBatchMutation func(exec sqlExecutor, accountIDs []int64) (affected int64, groupIDs []int64, err error)

func (r *accountRepository) withFencedAccountGroupBatch(
	ctx context.Context,
	accountIDs []int64,
	lockGroupIDs []int64,
	resolve accountGroupBatchResolver,
	mutate accountGroupBatchMutation,
) (int64, error) {
	if mutate == nil {
		return 0, nil
	}
	accountIDs = uniquePositiveInt64s(accountIDs)
	if len(accountIDs) == 0 && resolve == nil {
		return 0, nil
	}

	var tx *dbent.Tx
	var exec sqlExecutor
	ownsTx := false
	if ambientTx := dbent.TxFromContext(ctx); ambientTx != nil {
		exec = ambientTx.Client()
	} else {
		var err error
		tx, err = r.client.Tx(ctx)
		if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
			return 0, err
		}
		if err == nil {
			defer func() { _ = tx.Rollback() }()
			exec = tx.Client()
			ownsTx = true
		} else {
			if r.schedulerMutationFenceEnabled() {
				return 0, errSchedulerMutationTxContextRequired
			}
			exec = r.sql
		}
	}
	if err := lockAccountGroupsForMutation(ctx, exec, lockGroupIDs); err != nil {
		return 0, err
	}
	if resolve != nil {
		var err error
		accountIDs, err = resolve(exec)
		if err != nil {
			return 0, err
		}
		accountIDs = uniquePositiveInt64s(accountIDs)
	}
	if len(accountIDs) == 0 {
		return 0, nil
	}

	activeTx := txOrAmbient(ctx, tx)
	tokens, err := r.beginSchedulerAccountMutationsForTx(ctx, activeTx, accountIDs)
	if err != nil {
		return 0, err
	}
	r.registerSchedulerAccountMutationHooks(activeTx, tokens)
	resolved := false
	if ownsTx {
		defer func() {
			if !resolved {
				r.rollbackSchedulerAccountMutation(tx, tokens, "account_group_batch_failed")
			}
		}()
	}

	affected, groupIDs, err := mutate(exec, accountIDs)
	if err != nil {
		return 0, err
	}
	payload := map[string]any{"account_ids": accountIDs}
	if groupIDs = uniquePositiveInt64s(groupIDs); len(groupIDs) > 0 {
		payload["group_ids"] = groupIDs
	}
	payload = withSchedulerEpochs(payload, tokens)
	if err := enqueueSchedulerOutbox(ctx, exec, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, payload); err != nil {
		return 0, fmt.Errorf("enqueue account group batch outbox: %w", err)
	}
	if ownsTx {
		commitErr := r.commitSchedulerAccountMutation(tx, tokens)
		resolved = true
		if commitErr != nil {
			return 0, commitErr
		}
	}
	if r.shouldLegacySyncSchedulerCache(ctx, ownsTx) {
		r.syncSchedulerAccountSnapshots(ctx, accountIDs)
	}
	return affected, nil
}

func lockAccountGroupsForMutation(ctx context.Context, exec sqlExecutor, groupIDs []int64) error {
	groupIDs = uniquePositiveInt64s(groupIDs)
	if len(groupIDs) == 0 {
		return nil
	}
	rows, err := exec.QueryContext(ctx, `
		SELECT id
		FROM groups
		WHERE id = ANY($1) AND deleted_at IS NULL
		ORDER BY id
		FOR UPDATE
	`, pq.Array(groupIDs))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var groupID int64
		if err := rows.Scan(&groupID); err != nil {
			return err
		}
	}
	return rows.Err()
}

func queryDistinctGroupAccountIDs(ctx context.Context, exec sqlExecutor, groupIDs []int64) ([]int64, error) {
	rows, err := exec.QueryContext(ctx, "SELECT DISTINCT account_id FROM account_groups WHERE group_id = ANY($1) ORDER BY account_id", pq.Array(groupIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var accountIDs []int64
	for rows.Next() {
		var accountID int64
		if err := rows.Scan(&accountID); err != nil {
			return nil, err
		}
		accountIDs = append(accountIDs, accountID)
	}
	return accountIDs, rows.Err()
}

func queryDistinctAccountGroupIDs(ctx context.Context, exec sqlExecutor, accountIDs []int64) ([]int64, error) {
	rows, err := exec.QueryContext(ctx, "SELECT DISTINCT group_id FROM account_groups WHERE account_id = ANY($1) ORDER BY group_id", pq.Array(accountIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var groupIDs []int64
	for rows.Next() {
		var groupID int64
		if err := rows.Scan(&groupID); err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, groupID)
	}
	return groupIDs, rows.Err()
}

func (r *accountRepository) ListSchedulable(ctx context.Context) ([]service.Account, error) {
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			activeOrPoolModeErrorStatusPredicate(),
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			nonPoolModeOverloadAvailablePredicate(now),
			nonPoolModeRateLimitAvailablePredicate(now),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]service.Account, error) {
	return r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status:      service.StatusActive,
		schedulable: true,
	})
}

func (r *accountRepository) ListSchedulableByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformEQ(platform),
			activeOrPoolModeErrorStatusPredicate(),
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			nonPoolModeOverloadAvailablePredicate(now),
			nonPoolModeRateLimitAvailablePredicate(now),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]service.Account, error) {
	// 单平台查询复用多平台逻辑，保持过滤条件与排序策略一致。
	return r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status:      service.StatusActive,
		schedulable: true,
		platforms:   []string{platform},
	})
}

func (r *accountRepository) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	if len(platforms) == 0 {
		return nil, nil
	}
	// 仅返回可调度的活跃账号，并过滤处于过载/限流窗口的账号。
	// 代理与分组信息统一在 accountsToService 中批量加载，避免 N+1 查询。
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformIn(platforms...),
			activeOrPoolModeErrorStatusPredicate(),
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			nonPoolModeOverloadAvailablePredicate(now),
			nonPoolModeRateLimitAvailablePredicate(now),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformEQ(platform),
			activeOrPoolModeErrorStatusPredicate(),
			dbaccount.SchedulableEQ(true),
			dbaccount.Not(dbaccount.HasAccountGroups()),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			nonPoolModeOverloadAvailablePredicate(now),
			nonPoolModeRateLimitAvailablePredicate(now),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]service.Account, error) {
	if len(platforms) == 0 {
		return nil, nil
	}
	now := time.Now()
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformIn(platforms...),
			activeOrPoolModeErrorStatusPredicate(),
			dbaccount.SchedulableEQ(true),
			dbaccount.Not(dbaccount.HasAccountGroups()),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			nonPoolModeOverloadAvailablePredicate(now),
			nonPoolModeRateLimitAvailablePredicate(now),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]service.Account, error) {
	if len(platforms) == 0 {
		return nil, nil
	}
	// 复用按分组查询逻辑，保证分组优先级 + 账号优先级的排序与筛选一致。
	return r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status:      service.StatusActive,
		schedulable: true,
		platforms:   platforms,
	})
}

func (r *accountRepository) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	now := time.Now()
	return r.withFencedAccountMutation(ctx, id, service.SchedulerOutboxEventAccountChanged, nil, func(client *dbent.Client) error {
		_, err := client.Account.Update().
			Where(dbaccount.IDEQ(id)).
			SetRateLimitedAt(now).
			SetRateLimitResetAt(resetAt).
			Save(ctx)
		return err
	})
}

func (r *accountRepository) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time, reason ...string) error {
	if scope == "" {
		return nil
	}
	now := time.Now().UTC()
	payload := map[string]string{
		"rate_limited_at":     now.Format(time.RFC3339),
		"rate_limit_reset_at": resetAt.UTC().Format(time.RFC3339),
	}
	if len(reason) > 0 {
		if value := strings.TrimSpace(reason[0]); value != "" {
			payload["reason"] = value
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return r.withFencedAccountMutation(ctx, id, service.SchedulerOutboxEventAccountChanged, nil, func(client *dbent.Client) error {
		result, err := client.ExecContext(ctx, `UPDATE accounts SET
			extra = jsonb_set(
				jsonb_set(COALESCE(extra, '{}'::jsonb), '{model_rate_limits}'::text[], COALESCE(extra->'model_rate_limits', '{}'::jsonb), true),
				ARRAY['model_rate_limits', $1]::text[],
				$2::jsonb,
				true
			),
			updated_at = NOW()
		WHERE id = $3 AND deleted_at IS NULL`,
			scope, raw, id)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return service.ErrAccountNotFound
		}
		return nil
	})
}

func (r *accountRepository) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	return r.withFencedAccountMutation(ctx, id, service.SchedulerOutboxEventAccountChanged, nil, func(client *dbent.Client) error {
		_, err := client.Account.Update().
			Where(dbaccount.IDEQ(id)).
			SetOverloadUntil(until).
			Save(ctx)
		return err
	})
}

func (r *accountRepository) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	return r.withFencedAccountMutation(ctx, id, service.SchedulerOutboxEventAccountChanged, nil, func(client *dbent.Client) error {
		_, err := client.ExecContext(ctx, `
			UPDATE accounts
			SET temp_unschedulable_until = $1,
				temp_unschedulable_reason = $2,
				updated_at = NOW()
			WHERE id = $3
				AND deleted_at IS NULL
				AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until < $1)
		`, until, reason, id)
		return err
	})
}

func (r *accountRepository) ClearTempUnschedulable(ctx context.Context, id int64) error {
	return r.withFencedAccountMutation(ctx, id, service.SchedulerOutboxEventAccountChanged, nil, func(client *dbent.Client) error {
		_, err := client.ExecContext(ctx, `
			UPDATE accounts
			SET temp_unschedulable_until = NULL,
				temp_unschedulable_reason = NULL,
				updated_at = NOW()
			WHERE id = $1
				AND deleted_at IS NULL
		`, id)
		return err
	})
}

func (r *accountRepository) ClearRateLimit(ctx context.Context, id int64) error {
	return r.withFencedAccountMutation(ctx, id, service.SchedulerOutboxEventAccountChanged, nil, func(client *dbent.Client) error {
		_, err := client.Account.Update().
			Where(dbaccount.IDEQ(id)).
			ClearRateLimitedAt().
			ClearRateLimitResetAt().
			ClearOverloadUntil().
			Save(ctx)
		return err
	})
}

func (r *accountRepository) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(
		ctx,
		"UPDATE accounts SET extra = COALESCE(extra, '{}'::jsonb) - 'antigravity_quota_scopes', updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL",
		id,
	)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue clear quota scopes failed: account=%d err=%v", id, err)
	}
	return nil
}

func (r *accountRepository) ClearModelRateLimits(ctx context.Context, id int64) error {
	return r.withFencedAccountMutation(ctx, id, service.SchedulerOutboxEventAccountChanged, nil, func(client *dbent.Client) error {
		result, err := client.ExecContext(ctx,
			"UPDATE accounts SET extra = COALESCE(extra, '{}'::jsonb) - 'model_rate_limits', updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL",
			id,
		)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return service.ErrAccountNotFound
		}
		return nil
	})
}

func (r *accountRepository) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	builder := r.client.Account.Update().
		Where(dbaccount.IDEQ(id)).
		SetSessionWindowStatus(status)
	if start != nil {
		builder.SetSessionWindowStart(*start)
	}
	if end != nil {
		builder.SetSessionWindowEnd(*end)
	}
	_, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	// 触发调度器缓存更新（仅当窗口时间有变化时）
	if start != nil || end != nil {
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
			logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue session window update failed: account=%d err=%v", id, err)
		}
	}
	return nil
}

func (r *accountRepository) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	return r.withFencedAccountMutation(ctx, id, service.SchedulerOutboxEventAccountChanged, nil, func(client *dbent.Client) error {
		_, err := client.Account.Update().
			Where(dbaccount.IDEQ(id)).
			SetSchedulable(schedulable).
			Save(ctx)
		return err
	})
}

func (r *accountRepository) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	result, err := r.sql.ExecContext(ctx, `
		UPDATE accounts
		SET schedulable = FALSE,
			updated_at = NOW()
		WHERE deleted_at IS NULL
			AND schedulable = TRUE
			AND auto_pause_on_expired = TRUE
			AND expires_at IS NOT NULL
			AND expires_at <= $1
	`, now)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if rows > 0 {
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventFullRebuild, nil, nil, nil); err != nil {
			logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue auto pause rebuild failed: err=%v", err)
		}
	}
	return rows, nil
}

func (r *accountRepository) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	// 使用 JSONB 合并操作实现原子更新，避免读-改-写的并发丢失更新问题
	payload, err := json.Marshal(updates)
	if err != nil {
		return err
	}

	if shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
		return r.withFencedAccountMutation(ctx, id, service.SchedulerOutboxEventAccountChanged, nil, func(client *dbent.Client) error {
			return updateAccountExtraJSON(ctx, client, id, payload)
		})
	}

	if err := updateAccountExtraJSON(ctx, clientFromContext(ctx, r.client), id, payload); err != nil {
		return err
	}
	// Observational fields do not open a mutation fence. The scheduler cache's
	// guarded writer prevents this refresh from overwriting a fenced full lease.
	r.syncSchedulerAccountSnapshot(ctx, id)
	return nil
}

func updateAccountExtraJSON(ctx context.Context, client *dbent.Client, id int64, payload []byte) error {
	result, err := client.ExecContext(
		ctx,
		"UPDATE accounts SET extra = COALESCE(extra, '{}'::jsonb) || $1::jsonb, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL",
		string(payload), id,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAccountNotFound
	}
	return nil
}

func shouldEnqueueSchedulerOutboxForExtraUpdates(updates map[string]any) bool {
	if len(updates) == 0 {
		return false
	}
	for key := range updates {
		if isSchedulerNeutralExtraKey(key) {
			continue
		}
		return true
	}
	return false
}

func isSchedulerNeutralExtraKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if _, ok := schedulerNeutralExtraKeys[key]; ok {
		return true
	}
	for _, prefix := range schedulerNeutralExtraKeyPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func (r *accountRepository) BulkUpdate(ctx context.Context, ids []int64, updates service.AccountBulkUpdate) (int64, error) {
	updatedIDs, err := r.BulkUpdateReturningIDs(ctx, ids, updates)
	return int64(len(updatedIDs)), err
}

// BulkUpdateReturningIDs performs one UPDATE ... RETURNING query so callers can
// distinguish missing/deleted accounts without per-account reads.
func (r *accountRepository) BulkUpdateReturningIDs(ctx context.Context, ids []int64, updates service.AccountBulkUpdate) ([]int64, error) {
	if err := service.ValidateAccountAetherWSExtra(updates.Extra); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	setClauses := make([]string, 0, 8)
	args := make([]any, 0, 8)

	idx := 1
	if updates.Name != nil {
		setClauses = append(setClauses, "name = $"+itoa(idx))
		args = append(args, *updates.Name)
		idx++
	}
	if updates.ProxyID != nil {
		// 0 表示清除代理（前端发送 0 而不是 null 来表达清除意图）
		if *updates.ProxyID == 0 {
			setClauses = append(setClauses, "proxy_id = NULL")
		} else {
			setClauses = append(setClauses, "proxy_id = $"+itoa(idx))
			args = append(args, *updates.ProxyID)
			idx++
		}
	}
	if updates.Concurrency != nil {
		setClauses = append(setClauses, "concurrency = $"+itoa(idx))
		args = append(args, *updates.Concurrency)
		idx++
	}
	if updates.Priority != nil {
		setClauses = append(setClauses, "priority = $"+itoa(idx))
		args = append(args, *updates.Priority)
		idx++
	}
	if updates.RateMultiplier != nil {
		setClauses = append(setClauses, "rate_multiplier = $"+itoa(idx))
		args = append(args, *updates.RateMultiplier)
		idx++
	}
	if updates.LoadFactor != nil {
		if *updates.LoadFactor <= 0 {
			setClauses = append(setClauses, "load_factor = NULL")
		} else {
			setClauses = append(setClauses, "load_factor = $"+itoa(idx))
			args = append(args, *updates.LoadFactor)
			idx++
		}
	}
	if updates.Status != nil {
		setClauses = append(setClauses, "status = $"+itoa(idx))
		args = append(args, *updates.Status)
		idx++
	}
	if updates.Schedulable != nil {
		setClauses = append(setClauses, "schedulable = $"+itoa(idx))
		args = append(args, *updates.Schedulable)
		idx++
	}
	// JSONB 需要合并而非覆盖，使用 raw SQL 保持旧行为。
	if len(updates.Credentials) > 0 {
		payload, err := json.Marshal(updates.Credentials)
		if err != nil {
			return nil, err
		}
		setClauses = append(setClauses, "credentials = COALESCE(credentials, '{}'::jsonb) || $"+itoa(idx)+"::jsonb")
		args = append(args, payload)
		idx++
	}
	if len(updates.Extra) > 0 {
		payload, err := json.Marshal(updates.Extra)
		if err != nil {
			return nil, err
		}
		parameter := "$" + itoa(idx) + "::jsonb"
		// aether_ws is a versioned nested object. Merge it server-side so one
		// bulk UPDATE preserves unknown per-account fields without an N+1 read.
		// Historical malformed values are normalized so this update repairs
		// them atomically as well.
		existingExtra := "(CASE WHEN jsonb_typeof(extra) = 'object' THEN extra ELSE '{}'::jsonb END)"
		existingAetherWS := "(CASE WHEN jsonb_typeof(" + existingExtra + "->'aether_ws') = 'object' THEN " + existingExtra + "->'aether_ws' ELSE '{}'::jsonb END)"
		incomingAetherWS := "(CASE WHEN jsonb_typeof(" + parameter + "->'aether_ws') = 'object' THEN " + parameter + "->'aether_ws' ELSE '{}'::jsonb END)"
		setClauses = append(setClauses,
			"extra = CASE WHEN "+parameter+" ? 'aether_ws' THEN "+
				"jsonb_set("+existingExtra+" || ("+parameter+" - 'aether_ws'), '{aether_ws}', "+
				existingAetherWS+" || "+incomingAetherWS+", true) "+
				"ELSE "+existingExtra+" || "+parameter+" END")
		args = append(args, payload)
		idx++
	}

	if len(setClauses) == 0 {
		return nil, nil
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	query := "UPDATE accounts SET " + joinClauses(setClauses, ", ") + " WHERE id = ANY($" + itoa(idx) + ") AND deleted_at IS NULL RETURNING id"
	args = append(args, pq.Array(ids))

	var tx *dbent.Tx
	var exec sqlExecutor
	ownsTx := false
	var err error
	if ambientTx := dbent.TxFromContext(ctx); ambientTx != nil {
		exec = ambientTx.Client()
	} else {
		tx, err = r.client.Tx(ctx)
		if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
			return nil, err
		}
		if err == nil {
			defer func() { _ = tx.Rollback() }()
			exec = tx.Client()
			ownsTx = true
		} else {
			if r.schedulerMutationFenceEnabled() {
				return nil, errSchedulerMutationTxContextRequired
			}
			exec = r.sql
		}
	}
	tokens, err := r.beginSchedulerAccountMutationsForTx(ctx, txOrAmbient(ctx, tx), ids)
	if err != nil {
		return nil, err
	}
	r.registerSchedulerAccountMutationHooks(txOrAmbient(ctx, tx), tokens)
	resolved := false
	if ownsTx {
		defer func() {
			if !resolved {
				r.rollbackSchedulerAccountMutation(tx, tokens, "account_bulk_failed")
			}
		}()
	}

	rows, err := exec.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	updatedIDs := make([]int64, 0, len(ids))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		updatedIDs = append(updatedIDs, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(updatedIDs) > 0 {
		updatedTokens := make(map[int64]int64, len(updatedIDs))
		for _, accountID := range updatedIDs {
			if epoch := tokens[accountID]; epoch > 0 {
				updatedTokens[accountID] = epoch
			}
		}
		payload := withSchedulerEpochs(map[string]any{"account_ids": updatedIDs}, updatedTokens)
		if err := enqueueSchedulerOutbox(ctx, exec, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, payload); err != nil {
			return nil, fmt.Errorf("enqueue bulk update outbox: %w", err)
		}
	}
	if ownsTx {
		commitErr := r.commitSchedulerAccountMutation(tx, tokens)
		resolved = true
		if commitErr != nil {
			return nil, commitErr
		}
	}
	if r.shouldLegacySyncSchedulerCache(ctx, ownsTx) {
		r.syncSchedulerAccountSnapshots(ctx, updatedIDs)
	}
	return updatedIDs, nil
}

type accountGroupQueryOptions struct {
	status      string
	schedulable bool
	platforms   []string // 允许的多个平台，空切片表示不进行平台过滤
}

func (r *accountRepository) queryAccountsByGroup(ctx context.Context, groupID int64, opts accountGroupQueryOptions) ([]service.Account, error) {
	q := r.client.AccountGroup.Query().
		Where(dbaccountgroup.GroupIDEQ(groupID))

	// 通过 account_groups 中间表查询账号，并按需叠加状态/平台/调度能力过滤。
	preds := make([]dbpredicate.Account, 0, 6)
	preds = append(preds, dbaccount.DeletedAtIsNil())
	if opts.status != "" {
		switch opts.status {
		case service.StatusActive:
			preds = append(preds, activeOrPoolModeErrorStatusPredicate())
		case service.StatusError:
			preds = append(preds, nonPoolModeErrorStatusPredicate())
		default:
			preds = append(preds, dbaccount.StatusEQ(opts.status))
		}
	}
	if len(opts.platforms) > 0 {
		preds = append(preds, dbaccount.PlatformIn(opts.platforms...))
	}
	if opts.schedulable {
		now := time.Now()
		preds = append(preds,
			dbaccount.SchedulableEQ(true),
			tempUnschedulablePredicate(),
			notExpiredPredicate(now),
			nonPoolModeOverloadAvailablePredicate(now),
			nonPoolModeRateLimitAvailablePredicate(now),
		)
	}

	if len(preds) > 0 {
		q = q.Where(dbaccountgroup.HasAccountWith(preds...))
	}

	groups, err := q.
		Order(
			dbaccountgroup.ByPriority(),
			dbaccountgroup.ByAccountField(dbaccount.FieldPriority),
		).
		WithAccount().
		All(ctx)
	if err != nil {
		return nil, err
	}

	orderedIDs := make([]int64, 0, len(groups))
	accountMap := make(map[int64]*dbent.Account, len(groups))
	for _, ag := range groups {
		if ag.Edges.Account == nil {
			continue
		}
		if _, exists := accountMap[ag.AccountID]; exists {
			continue
		}
		accountMap[ag.AccountID] = ag.Edges.Account
		orderedIDs = append(orderedIDs, ag.AccountID)
	}

	accounts := make([]*dbent.Account, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		if acc, ok := accountMap[id]; ok {
			accounts = append(accounts, acc)
		}
	}

	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) accountsToService(ctx context.Context, accounts []*dbent.Account) ([]service.Account, error) {
	if len(accounts) == 0 {
		return []service.Account{}, nil
	}

	accountIDs := make([]int64, 0, len(accounts))
	proxyIDs := make([]int64, 0, len(accounts))
	for _, acc := range accounts {
		accountIDs = append(accountIDs, acc.ID)
		if acc.ProxyID != nil {
			proxyIDs = append(proxyIDs, *acc.ProxyID)
		}
	}

	proxyMap, err := r.loadProxies(ctx, proxyIDs)
	if err != nil {
		return nil, err
	}
	groupsByAccount, groupIDsByAccount, accountGroupsByAccount, err := r.loadAccountGroups(ctx, accountIDs)
	if err != nil {
		return nil, err
	}

	outAccounts := make([]service.Account, 0, len(accounts))
	for _, acc := range accounts {
		out := accountEntityToService(acc)
		if out == nil {
			continue
		}
		if acc.ProxyID != nil {
			if proxy, ok := proxyMap[*acc.ProxyID]; ok {
				out.Proxy = proxy
			}
		}
		if groups, ok := groupsByAccount[acc.ID]; ok {
			out.Groups = groups
		}
		if groupIDs, ok := groupIDsByAccount[acc.ID]; ok {
			out.GroupIDs = groupIDs
		}
		if ags, ok := accountGroupsByAccount[acc.ID]; ok {
			out.AccountGroups = ags
		}
		outAccounts = append(outAccounts, *out)
	}

	return outAccounts, nil
}

func tempUnschedulablePredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		col := s.C("temp_unschedulable_until")
		s.Where(entsql.Or(
			entsql.IsNull(col),
			entsql.LTE(col, entsql.Expr("NOW()")),
		))
	})
}

func notExpiredPredicate(now time.Time) dbpredicate.Account {
	return dbaccount.Or(
		dbaccount.ExpiresAtIsNil(),
		dbaccount.ExpiresAtGT(now),
		dbaccount.AutoPauseOnExpiredEQ(false),
	)
}

func nonPoolModeRateLimitAvailablePredicate(now time.Time) dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		rateLimitCol := s.C("rate_limit_reset_at")
		poolModePath := sqljson.Path("pool_mode")
		poolModeEnabled := entsql.Or(
			sqljson.ValueEQ(dbaccount.FieldCredentials, true, poolModePath),
			sqljson.ValueEQ(dbaccount.FieldCredentials, "true", poolModePath),
		)
		s.Where(entsql.Or(
			entsql.IsNull(rateLimitCol),
			entsql.LTE(rateLimitCol, entsql.Expr("NOW()")),
			poolModeEnabled,
		))
	})
}

func nonPoolModeRateLimitedPredicate(now time.Time) dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		rateLimitCol := s.C("rate_limit_reset_at")
		poolModePath := sqljson.Path("pool_mode")
		s.Where(entsql.And(
			entsql.Not(entsql.IsNull(rateLimitCol)),
			entsql.GT(rateLimitCol, entsql.Expr("NOW()")),
			entsql.Or(
				entsql.Not(sqljson.HasKey(dbaccount.FieldCredentials, poolModePath)),
				sqljson.ValueEQ(dbaccount.FieldCredentials, false, poolModePath),
				sqljson.ValueEQ(dbaccount.FieldCredentials, "false", poolModePath),
			),
		))
	})
}

func activeOrPoolModeErrorStatusPredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		statusCol := s.C("status")
		poolModePath := sqljson.Path("pool_mode")
		s.Where(entsql.Or(
			entsql.EQ(statusCol, service.StatusActive),
			entsql.And(
				entsql.EQ(statusCol, service.StatusError),
				sqljson.ValueEQ(dbaccount.FieldCredentials, true, poolModePath),
			),
		))
	})
}

func nonPoolModeOverloadAvailablePredicate(now time.Time) dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		overloadCol := s.C("overload_until")
		poolModePath := sqljson.Path("pool_mode")
		poolModeEnabled := entsql.Or(
			sqljson.ValueEQ(dbaccount.FieldCredentials, true, poolModePath),
			sqljson.ValueEQ(dbaccount.FieldCredentials, "true", poolModePath),
		)
		s.Where(entsql.Or(
			entsql.IsNull(overloadCol),
			entsql.LTE(overloadCol, entsql.Expr("NOW()")),
			poolModeEnabled,
		))
	})
}

func nonPoolModeErrorStatusPredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		statusCol := s.C("status")
		poolModePath := sqljson.Path("pool_mode")
		s.Where(entsql.And(
			entsql.EQ(statusCol, service.StatusError),
			entsql.Or(
				entsql.Not(sqljson.HasKey(dbaccount.FieldCredentials, poolModePath)),
				sqljson.ValueEQ(dbaccount.FieldCredentials, false, poolModePath),
				sqljson.ValueEQ(dbaccount.FieldCredentials, "false", poolModePath),
			),
		))
	})
}

func (r *accountRepository) loadProxies(ctx context.Context, proxyIDs []int64) (map[int64]*service.Proxy, error) {
	proxyMap := make(map[int64]*service.Proxy)
	if len(proxyIDs) == 0 {
		return proxyMap, nil
	}

	proxies, err := r.client.Proxy.Query().Where(dbproxy.IDIn(proxyIDs...)).All(ctx)
	if err != nil {
		return nil, err
	}

	for _, p := range proxies {
		proxyMap[p.ID] = proxyEntityToService(p)
	}
	return proxyMap, nil
}

func (r *accountRepository) loadAccountGroups(ctx context.Context, accountIDs []int64) (map[int64][]*service.Group, map[int64][]int64, map[int64][]service.AccountGroup, error) {
	groupsByAccount := make(map[int64][]*service.Group)
	groupIDsByAccount := make(map[int64][]int64)
	accountGroupsByAccount := make(map[int64][]service.AccountGroup)

	if len(accountIDs) == 0 {
		return groupsByAccount, groupIDsByAccount, accountGroupsByAccount, nil
	}

	entries, err := r.client.AccountGroup.Query().
		Where(dbaccountgroup.AccountIDIn(accountIDs...)).
		WithGroup().
		Order(dbaccountgroup.ByAccountID(), dbaccountgroup.ByPriority()).
		All(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	for _, ag := range entries {
		groupSvc := groupEntityToService(ag.Edges.Group)
		agSvc := service.AccountGroup{
			AccountID: ag.AccountID,
			GroupID:   ag.GroupID,
			Priority:  ag.Priority,
			CreatedAt: ag.CreatedAt,
			Group:     groupSvc,
		}
		accountGroupsByAccount[ag.AccountID] = append(accountGroupsByAccount[ag.AccountID], agSvc)
		groupIDsByAccount[ag.AccountID] = append(groupIDsByAccount[ag.AccountID], ag.GroupID)
		if groupSvc != nil {
			groupsByAccount[ag.AccountID] = append(groupsByAccount[ag.AccountID], groupSvc)
		}
	}

	return groupsByAccount, groupIDsByAccount, accountGroupsByAccount, nil
}

func (r *accountRepository) loadAccountGroupIDs(ctx context.Context, accountID int64) ([]int64, error) {
	return loadAccountGroupIDsWithClient(ctx, clientFromContext(ctx, r.client), accountID)
}

func loadAccountGroupIDsWithClient(ctx context.Context, client *dbent.Client, accountID int64) ([]int64, error) {
	entries, err := client.AccountGroup.
		Query().
		Where(dbaccountgroup.AccountIDEQ(accountID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.GroupID)
	}
	return ids, nil
}

func mergeGroupIDs(a []int64, b []int64) []int64 {
	seen := make(map[int64]struct{}, len(a)+len(b))
	out := make([]int64, 0, len(a)+len(b))
	for _, id := range a {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range b {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func buildSchedulerGroupPayload(groupIDs []int64) map[string]any {
	if len(groupIDs) == 0 {
		return nil
	}
	return map[string]any{"group_ids": groupIDs}
}

func accountEntityToService(m *dbent.Account) *service.Account {
	if m == nil {
		return nil
	}

	rateMultiplier := m.RateMultiplier

	account := &service.Account{
		ID:                      m.ID,
		Name:                    m.Name,
		Notes:                   m.Notes,
		Platform:                m.Platform,
		Type:                    m.Type,
		Credentials:             copyJSONMap(m.Credentials),
		Extra:                   copyJSONMap(m.Extra),
		ProxyID:                 m.ProxyID,
		Concurrency:             m.Concurrency,
		Priority:                m.Priority,
		RateMultiplier:          &rateMultiplier,
		LoadFactor:              m.LoadFactor,
		Status:                  m.Status,
		ErrorMessage:            derefString(m.ErrorMessage),
		LastUsedAt:              m.LastUsedAt,
		ExpiresAt:               m.ExpiresAt,
		AutoPauseOnExpired:      m.AutoPauseOnExpired,
		CreatedAt:               m.CreatedAt,
		UpdatedAt:               m.UpdatedAt,
		Schedulable:             m.Schedulable,
		RateLimitedAt:           m.RateLimitedAt,
		RateLimitResetAt:        m.RateLimitResetAt,
		OverloadUntil:           m.OverloadUntil,
		TempUnschedulableUntil:  m.TempUnschedulableUntil,
		TempUnschedulableReason: derefString(m.TempUnschedulableReason),
		SessionWindowStart:      m.SessionWindowStart,
		SessionWindowEnd:        m.SessionWindowEnd,
		SessionWindowStatus:     derefString(m.SessionWindowStatus),
	}
	if account.IsPoolMode() {
		account.Status = account.EffectiveStatus()
		account.ErrorMessage = ""
		account.OverloadUntil = nil
	}
	return account
}

func normalizeJSONMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}

func copyJSONMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func joinClauses(clauses []string, sep string) string {
	if len(clauses) == 0 {
		return ""
	}
	out := clauses[0]
	for i := 1; i < len(clauses); i++ {
		out += sep + clauses[i]
	}
	return out
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

// FindByExtraField 根据 extra 字段中的键值对查找账号。
// 使用 PostgreSQL JSONB @> 操作符进行高效查询（需要 GIN 索引支持）。
//
// FindByExtraField finds accounts by key-value pairs in the extra field.
// Uses PostgreSQL JSONB @> operator for efficient queries (requires GIN index).
func (r *accountRepository) FindByExtraField(ctx context.Context, key string, value any) ([]service.Account, error) {
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.DeletedAtIsNil(),
			func(s *entsql.Selector) {
				path := sqljson.Path(key)
				switch v := value.(type) {
				case string:
					preds := []*entsql.Predicate{sqljson.ValueEQ(dbaccount.FieldExtra, v, path)}
					if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
						preds = append(preds, sqljson.ValueEQ(dbaccount.FieldExtra, parsed, path))
					}
					if len(preds) == 1 {
						s.Where(preds[0])
					} else {
						s.Where(entsql.Or(preds...))
					}
				case int:
					s.Where(entsql.Or(
						sqljson.ValueEQ(dbaccount.FieldExtra, v, path),
						sqljson.ValueEQ(dbaccount.FieldExtra, strconv.Itoa(v), path),
					))
				case int64:
					s.Where(entsql.Or(
						sqljson.ValueEQ(dbaccount.FieldExtra, v, path),
						sqljson.ValueEQ(dbaccount.FieldExtra, strconv.FormatInt(v, 10), path),
					))
				case json.Number:
					if parsed, err := v.Int64(); err == nil {
						s.Where(entsql.Or(
							sqljson.ValueEQ(dbaccount.FieldExtra, parsed, path),
							sqljson.ValueEQ(dbaccount.FieldExtra, v.String(), path),
						))
					} else {
						s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, v.String(), path))
					}
				default:
					s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, value, path))
				}
			},
		).
		All(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}

	return r.accountsToService(ctx, accounts)
}

// nowUTC is a SQL expression to generate a UTC RFC3339 timestamp string.
const nowUTC = `to_char(NOW() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`

// dailyExpiredExpr is a SQL expression that evaluates to TRUE when daily quota period has expired.
// Supports both rolling (24h from start) and fixed (pre-computed reset_at) modes.
const dailyExpiredExpr = `(
	CASE WHEN COALESCE(extra->>'quota_daily_reset_mode', 'rolling') = 'fixed'
	THEN NOW() >= COALESCE((extra->>'quota_daily_reset_at')::timestamptz, '1970-01-01'::timestamptz)
	ELSE COALESCE((extra->>'quota_daily_start')::timestamptz, '1970-01-01'::timestamptz)
		+ '24 hours'::interval <= NOW()
	END
)`

// weeklyExpiredExpr is a SQL expression that evaluates to TRUE when weekly quota period has expired.
const weeklyExpiredExpr = `(
	CASE WHEN COALESCE(extra->>'quota_weekly_reset_mode', 'rolling') = 'fixed'
	THEN NOW() >= COALESCE((extra->>'quota_weekly_reset_at')::timestamptz, '1970-01-01'::timestamptz)
	ELSE COALESCE((extra->>'quota_weekly_start')::timestamptz, '1970-01-01'::timestamptz)
		+ '168 hours'::interval <= NOW()
	END
)`

// nextDailyResetAtExpr is a SQL expression to compute the next daily reset_at when a reset occurs.
// For fixed mode: computes the next future reset time based on NOW(), timezone, and configured hour.
// This correctly handles long-inactive accounts by jumping directly to the next valid reset point.
const nextDailyResetAtExpr = `(
	CASE WHEN COALESCE(extra->>'quota_daily_reset_mode', 'rolling') = 'fixed'
	THEN to_char((
		-- Compute today's reset point in the configured timezone, then pick next future one
		CASE WHEN NOW() >= (
			date_trunc('day', NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))
			+ (COALESCE((extra->>'quota_daily_reset_hour')::int, 0) || ' hours')::interval
		) AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC')
		-- NOW() is at or past today's reset point → next reset is tomorrow
		THEN (
			date_trunc('day', NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))
			+ (COALESCE((extra->>'quota_daily_reset_hour')::int, 0) || ' hours')::interval
			+ '1 day'::interval
		) AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC')
		-- NOW() is before today's reset point → next reset is today
		ELSE (
			date_trunc('day', NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))
			+ (COALESCE((extra->>'quota_daily_reset_hour')::int, 0) || ' hours')::interval
		) AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC')
		END
	) AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
	ELSE NULL END
)`

// nextWeeklyResetAtExpr is a SQL expression to compute the next weekly reset_at when a reset occurs.
// For fixed mode: computes the next future reset time based on NOW(), timezone, configured day and hour.
// This correctly handles long-inactive accounts by jumping directly to the next valid reset point.
const nextWeeklyResetAtExpr = `(
	CASE WHEN COALESCE(extra->>'quota_weekly_reset_mode', 'rolling') = 'fixed'
	THEN to_char((
		-- Compute this week's reset point in the configured timezone
		-- Step 1: get today's date at reset hour in configured tz
		-- Step 2: compute days forward to target weekday
		-- Step 3: if same day but past reset hour, advance 7 days
		CASE
		WHEN (
			-- days_forward = (target_day - current_day + 7) % 7
			(COALESCE((extra->>'quota_weekly_reset_day')::int, 1)
			 - EXTRACT(DOW FROM NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))::int
			 + 7) % 7
		) = 0 AND NOW() >= (
			date_trunc('day', NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))
			+ (COALESCE((extra->>'quota_weekly_reset_hour')::int, 0) || ' hours')::interval
		) AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC')
		-- Same weekday and past reset hour → next week
		THEN (
			date_trunc('day', NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))
			+ (COALESCE((extra->>'quota_weekly_reset_hour')::int, 0) || ' hours')::interval
			+ '7 days'::interval
		) AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC')
		ELSE (
			-- Advance to target weekday this week (or next if days_forward > 0)
			date_trunc('day', NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))
			+ (COALESCE((extra->>'quota_weekly_reset_hour')::int, 0) || ' hours')::interval
			+ ((
				(COALESCE((extra->>'quota_weekly_reset_day')::int, 1)
				 - EXTRACT(DOW FROM NOW() AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC'))::int
				 + 7) % 7
			) || ' days')::interval
		) AT TIME ZONE COALESCE(extra->>'quota_reset_timezone', 'UTC')
		END
	) AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
	ELSE NULL END
)`

// IncrementQuotaUsed 原子递增账号的配额用量（总/日/周三个维度）
// 日/周额度在周期过期时自动重置为 0 再递增。
// 支持滚动窗口（rolling）和固定时间（fixed）两种重置模式。
func (r *accountRepository) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error {
	rows, err := r.sql.QueryContext(ctx,
		`UPDATE accounts SET extra = (
			COALESCE(extra, '{}'::jsonb)
			-- 总额度：始终递增
			|| jsonb_build_object('quota_used', COALESCE((extra->>'quota_used')::numeric, 0) + $1)
			-- 日额度：仅在 quota_daily_limit > 0 时处理
			|| CASE WHEN COALESCE((extra->>'quota_daily_limit')::numeric, 0) > 0 THEN
				jsonb_build_object(
					'quota_daily_used',
					CASE WHEN `+dailyExpiredExpr+`
					THEN $1
					ELSE COALESCE((extra->>'quota_daily_used')::numeric, 0) + $1 END,
					'quota_daily_start',
					CASE WHEN `+dailyExpiredExpr+`
					THEN `+nowUTC+`
					ELSE COALESCE(extra->>'quota_daily_start', `+nowUTC+`) END
				)
				-- 固定模式重置时更新下次重置时间
				|| CASE WHEN `+dailyExpiredExpr+` AND `+nextDailyResetAtExpr+` IS NOT NULL
				   THEN jsonb_build_object('quota_daily_reset_at', `+nextDailyResetAtExpr+`)
				   ELSE '{}'::jsonb END
			ELSE '{}'::jsonb END
			-- 周额度：仅在 quota_weekly_limit > 0 时处理
			|| CASE WHEN COALESCE((extra->>'quota_weekly_limit')::numeric, 0) > 0 THEN
				jsonb_build_object(
					'quota_weekly_used',
					CASE WHEN `+weeklyExpiredExpr+`
					THEN $1
					ELSE COALESCE((extra->>'quota_weekly_used')::numeric, 0) + $1 END,
					'quota_weekly_start',
					CASE WHEN `+weeklyExpiredExpr+`
					THEN `+nowUTC+`
					ELSE COALESCE(extra->>'quota_weekly_start', `+nowUTC+`) END
				)
				-- 固定模式重置时更新下次重置时间
				|| CASE WHEN `+weeklyExpiredExpr+` AND `+nextWeeklyResetAtExpr+` IS NOT NULL
				   THEN jsonb_build_object('quota_weekly_reset_at', `+nextWeeklyResetAtExpr+`)
				   ELSE '{}'::jsonb END
			ELSE '{}'::jsonb END
		), updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING
			COALESCE((extra->>'quota_used')::numeric, 0),
			COALESCE((extra->>'quota_limit')::numeric, 0)`,
		amount, id)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	var newUsed, limit float64
	if rows.Next() {
		if err := rows.Scan(&newUsed, &limit); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// 任一维度配额刚超限时触发调度快照刷新
	if limit > 0 && newUsed >= limit && (newUsed-amount) < limit {
		if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
			logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue quota exceeded failed: account=%d err=%v", id, err)
		}
	}
	return nil
}

// ResetQuotaUsed 重置账号所有维度的配额用量为 0
// 保留固定重置模式的配置字段（quota_daily_reset_mode 等），仅清零用量和窗口起始时间
func (r *accountRepository) ResetQuotaUsed(ctx context.Context, id int64) error {
	return r.withFencedAccountMutation(ctx, id, service.SchedulerOutboxEventAccountChanged, nil, func(client *dbent.Client) error {
		_, err := client.ExecContext(ctx,
			`UPDATE accounts SET extra = (
				COALESCE(extra, '{}'::jsonb)
				|| '{"quota_used": 0, "quota_daily_used": 0, "quota_weekly_used": 0}'::jsonb
			) - 'quota_daily_start' - 'quota_weekly_start' - 'quota_daily_reset_at' - 'quota_weekly_reset_at', updated_at = NOW()
			WHERE id = $1 AND deleted_at IS NULL`,
			id)
		return err
	})
}
