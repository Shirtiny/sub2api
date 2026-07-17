package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	// The v2 namespace is intentional: pre-version binaries unconditionally SET
	// sched:acc/sched:meta and also advance the shared snapshot keys. A disjoint
	// namespace keeps those writers from breaking the payload/version invariant
	// during a rolling binary replacement. The durable outbox watermark remains
	// shared so an upgrade does not replay its unbounded history. After every
	// pre-v2 writer is drained, a final rebuild must hydrate this namespace before
	// retained WS is enabled.
	schedulerBucketSetKey         = "sched:v2:buckets"
	schedulerOutboxWatermarkKey   = "sched:outbox:watermark"
	schedulerAccountPrefix        = "sched:v2:acc:"
	schedulerAccountMetaPrefix    = "sched:v2:meta:"
	schedulerAccountEpochPrefix   = "sched:v2:acc:epoch:"
	schedulerAccountVersionPrefix = "sched:v2:acc:version:"
	schedulerAccountFencePrefix   = "sched:v2:acc:pending:"
	schedulerAccountTombPrefix    = "sched:v2:acc:tombstone:"
	schedulerActivePrefix         = "sched:v2:active:"
	schedulerReadyPrefix          = "sched:v2:ready:"
	schedulerVersionPrefix        = "sched:v2:ver:"
	schedulerSnapshotPrefix       = "sched:v2:"
	schedulerLockPrefix           = "sched:v2:lock:"

	defaultSchedulerSnapshotMGetChunkSize  = 128
	defaultSchedulerSnapshotWriteChunkSize = 256

	// snapshotGraceTTLSeconds 旧快照过期的宽限期（秒）。
	// 替代立即 DEL，让正在读取旧版本的 reader 有足够时间完成 ZRANGE。
	snapshotGraceTTLSeconds = 60

	beginAccountMutationLua = `
if redis.call('EXISTS', KEYS[2]) == 1 then
	return 0
end
local epoch = redis.call('INCR', KEYS[1])
redis.call('PSETEX', KEYS[2], ARGV[1], tostring(epoch))
redis.call('DEL', KEYS[3])
redis.call('DEL', KEYS[4])
return epoch
`

	guardedSetAccountLua = `
if redis.call('EXISTS', KEYS[1]) == 1 then
	return 0
end
if redis.call('EXISTS', KEYS[3]) == 1 then
	return 0
end
local epochExists = redis.call('EXISTS', KEYS[2]) == 1
local currentVersion = redis.call('GET', KEYS[6])
if currentVersion ~= false then
	if ARGV[3] < currentVersion or (epochExists and ARGV[3] == currentVersion) then
		if ARGV[4] ~= '0' then
			-- updated_at is not a commit-order clock across application and SQL
			-- writers. Ask the caller to WATCH-merge only restrictive quota fields
			-- into the newer payload; copying this entire stale account could restore
			-- old credentials/status after a daily/weekly window expires.
			return 3
		end
		return 0
	end
elseif epochExists and redis.call('EXISTS', KEYS[4]) == 1 then
	-- A full value written by an older binary after a fenced mutation has no
	-- comparable source version. Ask the caller to take the WATCH-based legacy
	-- upgrade path instead of allowing an unversioned stale overwrite.
	return 2
end
redis.call('SET', KEYS[4], ARGV[1])
redis.call('SET', KEYS[5], ARGV[2])
redis.call('SET', KEYS[6], ARGV[3])
return 1
`

	compareAndSetAccountLua = `
if redis.call('EXISTS', KEYS[1]) == 1 or redis.call('EXISTS', KEYS[2]) == 1 then
	return 0
end
local current = redis.call('GET', KEYS[3])
if current == false or current ~= ARGV[1] then
	return 0
end
redis.call('SET', KEYS[3], ARGV[2])
redis.call('SET', KEYS[4], ARGV[3])
return 1
`

	publishAccountMutationLua = `
local current = redis.call('GET', KEYS[1])
if current == false or current ~= ARGV[1] then
	return 0
end
-- Begin deletes any older tombstone before allocating this epoch. A tombstone
-- that is still present therefore finalized this same epoch as a deletion;
-- deletion is the restrictive terminal outcome and cannot be resurrected by a
-- delayed publisher.
if redis.call('EXISTS', KEYS[3]) == 1 then
	return 0
end
local pending = redis.call('GET', KEYS[2])
if pending ~= false and pending ~= ARGV[1] then
	return 0
end
local publishedVersion = ARGV[4]
local currentVersion = redis.call('GET', KEYS[6])
local preserveCurrentValue = currentVersion ~= false
	and pending == false
	and (currentVersion > publishedVersion
		or (currentVersion == publishedVersion and redis.call('EXISTS', KEYS[4]) == 1))
if preserveCurrentValue then
	-- The fence expired and an ordinary DB refresh installed a newer revision,
	-- or this epoch already reached its first published terminal value. Acknowledge
	-- the epoch without relabeling/overwriting that value. Equal-version quota
	-- enforcement merged after the first publish must also remain intact.
	publishedVersion = currentVersion
else
	redis.call('SET', KEYS[4], ARGV[2])
	redis.call('SET', KEYS[5], ARGV[3])
	if currentVersion ~= false and currentVersion > publishedVersion then
		publishedVersion = currentVersion
	end
	redis.call('SET', KEYS[6], publishedVersion)
end
if pending == ARGV[1] then
	redis.call('DEL', KEYS[2])
end
return 1
`

	completeAccountDeletionLua = `
local current = redis.call('GET', KEYS[1])
if current == false or current ~= ARGV[1] then
	return 0
end
local pending = redis.call('GET', KEYS[2])
if pending ~= false and pending ~= ARGV[1] then
	return 0
end
redis.call('SET', KEYS[3], ARGV[1])
redis.call('DEL', KEYS[4], KEYS[5], KEYS[6])
if pending == ARGV[1] then
	redis.call('DEL', KEYS[2])
end
return 1
`
)

var (
	getAccountLeaseScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 or redis.call('EXISTS', KEYS[2]) == 1 then
	return false
end
return redis.call('GET', KEYS[3])
`)

	publishAccountMutationScript = redis.NewScript(publishAccountMutationLua)

	completeAccountDeletionScript = redis.NewScript(completeAccountDeletionLua)

	// activateSnapshotScript 原子 CAS 切换快照版本。
	// 仅当新版本号 >= 当前激活版本时才切换，防止并发写入导致版本回滚。
	// 旧快照使用 EXPIRE 设置宽限期而非立即 DEL，避免与 reader 竞态。
	//
	// KEYS[1] = activeKey     (sched:v2:active:{bucket})
	// KEYS[2] = readyKey      (sched:v2:ready:{bucket})
	// KEYS[3] = bucketSetKey  (sched:v2:buckets)
	// KEYS[4] = snapshotKey   (新写入的快照 key)
	// ARGV[1] = 新版本号字符串
	// ARGV[2] = bucket 字符串 (用于 SADD)
	// ARGV[3] = 快照 key 前缀 (用于构造旧快照 key)
	// ARGV[4] = 宽限期 TTL 秒数
	//
	// 返回 1 = 已激活, 0 = 版本过旧未激活
	activateSnapshotScript = redis.NewScript(`
local currentActive = redis.call('GET', KEYS[1])
local newVersion = tonumber(ARGV[1])

if currentActive ~= false then
	local curVersion = tonumber(currentActive)
	if curVersion and newVersion < curVersion then
		redis.call('DEL', KEYS[4])
		return 0
	end
end

redis.call('SET', KEYS[1], ARGV[1])
redis.call('SET', KEYS[2], '1')
redis.call('SADD', KEYS[3], ARGV[2])

if currentActive ~= false and currentActive ~= ARGV[1] then
	redis.call('EXPIRE', ARGV[3] .. currentActive, tonumber(ARGV[4]))
end

return 1
`)
)

type schedulerCache struct {
	rdb            *redis.Client
	mgetChunkSize  int
	writeChunkSize int
}

func NewSchedulerCache(rdb *redis.Client) service.SchedulerCache {
	return newSchedulerCacheWithChunkSizes(rdb, defaultSchedulerSnapshotMGetChunkSize, defaultSchedulerSnapshotWriteChunkSize)
}

func newSchedulerCacheWithChunkSizes(rdb *redis.Client, mgetChunkSize, writeChunkSize int) service.SchedulerCache {
	if mgetChunkSize <= 0 {
		mgetChunkSize = defaultSchedulerSnapshotMGetChunkSize
	}
	if writeChunkSize <= 0 {
		writeChunkSize = defaultSchedulerSnapshotWriteChunkSize
	}
	return &schedulerCache{
		rdb:            rdb,
		mgetChunkSize:  mgetChunkSize,
		writeChunkSize: writeChunkSize,
	}
}

func (c *schedulerCache) GetSnapshot(ctx context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	readyKey := schedulerBucketKey(schedulerReadyPrefix, bucket)
	readyVal, err := c.rdb.Get(ctx, readyKey).Result()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if readyVal != "1" {
		return nil, false, nil
	}

	activeKey := schedulerBucketKey(schedulerActivePrefix, bucket)
	activeVal, err := c.rdb.Get(ctx, activeKey).Result()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	snapshotKey := schedulerSnapshotKey(bucket, activeVal)
	ids, err := c.rdb.ZRange(ctx, snapshotKey, 0, -1).Result()
	if err != nil {
		return nil, false, err
	}
	if len(ids) == 0 {
		// 空快照视为缓存未命中，触发数据库回退查询
		// 这解决了新分组创建后立即绑定账号时的竞态条件问题
		return nil, false, nil
	}

	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, schedulerAccountMetaKey(id))
	}
	values, err := c.mgetChunked(ctx, keys)
	if err != nil {
		return nil, false, err
	}

	accounts := make([]*service.Account, 0, len(values))
	for _, val := range values {
		if val == nil {
			return nil, false, nil
		}
		account, err := decodeCachedAccount(val)
		if err != nil {
			return nil, false, err
		}
		accounts = append(accounts, account)
	}

	return accounts, true, nil
}

func (c *schedulerCache) SetSnapshot(ctx context.Context, bucket service.SchedulerBucket, accounts []service.Account) error {
	// Phase 1: 分配新版本号并写入快照数据。
	// INCR 保证每个调用方获得唯一递增版本号。
	// 写入的 snapshotKey 是新的版本化 key，reader 尚不知晓，因此无竞态。
	versionKey := schedulerBucketKey(schedulerVersionPrefix, bucket)
	version, err := c.rdb.Incr(ctx, versionKey).Result()
	if err != nil {
		return err
	}

	versionStr := strconv.FormatInt(version, 10)
	snapshotKey := schedulerSnapshotKey(bucket, versionStr)

	if err := c.writeAccounts(ctx, accounts); err != nil {
		return err
	}

	if len(accounts) > 0 {
		// 使用序号作为 score，保持数据库返回的排序语义。
		members := make([]redis.Z, 0, len(accounts))
		for idx, account := range accounts {
			members = append(members, redis.Z{
				Score:  float64(idx),
				Member: strconv.FormatInt(account.ID, 10),
			})
		}
		pipe := c.rdb.Pipeline()
		for start := 0; start < len(members); start += c.writeChunkSize {
			end := start + c.writeChunkSize
			if end > len(members) {
				end = len(members)
			}
			pipe.ZAdd(ctx, snapshotKey, members[start:end]...)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
	}

	// Phase 2: 原子 CAS 激活版本。
	// Lua 脚本保证：仅当新版本 >= 当前激活版本时才切换 active 指针，
	// 防止并发写入导致版本回滚。
	// 旧快照使用 EXPIRE 宽限期而非立即 DEL，避免 reader 竞态。
	activeKey := schedulerBucketKey(schedulerActivePrefix, bucket)
	readyKey := schedulerBucketKey(schedulerReadyPrefix, bucket)
	snapshotKeyPrefix := fmt.Sprintf("%s%d:%s:%s:v", schedulerSnapshotPrefix, bucket.GroupID, bucket.Platform, bucket.Mode)

	keys := []string{activeKey, readyKey, schedulerBucketSetKey, snapshotKey}
	args := []any{versionStr, bucket.String(), snapshotKeyPrefix, snapshotGraceTTLSeconds}

	_, err = activateSnapshotScript.Run(ctx, c.rdb, keys, args...).Result()
	if err != nil {
		return err
	}

	return nil
}

func (c *schedulerCache) GetAccount(ctx context.Context, accountID int64) (*service.Account, error) {
	id := strconv.FormatInt(accountID, 10)
	val, err := getAccountLeaseScript.Run(ctx, c.rdb, []string{
		schedulerAccountFenceKey(id),
		schedulerAccountTombstoneKey(id),
		schedulerAccountKey(id),
	}).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeCachedAccount(val)
}

func (c *schedulerCache) SetAccount(ctx context.Context, account *service.Account) error {
	if account == nil || account.ID <= 0 {
		return nil
	}
	return c.writeAccounts(ctx, []service.Account{*account})
}

func (c *schedulerCache) BeginAccountMutations(ctx context.Context, accountIDs []int64, ttl time.Duration) (map[int64]int64, error) {
	ids := uniquePositiveSchedulerAccountIDs(accountIDs)
	if len(ids) == 0 {
		return map[int64]int64{}, nil
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("scheduler account mutation fence ttl must be positive")
	}

	pipe := c.rdb.Pipeline()
	commands := make(map[int64]*redis.Cmd, len(ids))
	for _, accountID := range ids {
		id := strconv.FormatInt(accountID, 10)
		commands[accountID] = pipe.Eval(ctx, beginAccountMutationLua, []string{
			schedulerAccountEpochKey(id),
			schedulerAccountFenceKey(id),
			schedulerAccountKey(id),
			schedulerAccountTombstoneKey(id),
		}, ttl.Milliseconds())
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}

	tokens := make(map[int64]int64, len(commands))
	var conflict bool
	for accountID, command := range commands {
		epoch, err := command.Int64()
		if err != nil {
			return tokens, err
		}
		if epoch <= 0 {
			conflict = true
			continue
		}
		tokens[accountID] = epoch
	}
	if conflict {
		return tokens, service.ErrSchedulerAccountMutationInProgress
	}
	return tokens, nil
}

func (c *schedulerCache) PublishAccountMutation(ctx context.Context, account *service.Account, epoch int64) (bool, error) {
	if account == nil || account.ID <= 0 || epoch <= 0 {
		return false, nil
	}
	fullPayload, metaPayload, err := encodeSchedulerAccount(*account)
	if err != nil {
		return false, err
	}
	id := strconv.FormatInt(account.ID, 10)
	result, err := publishAccountMutationScript.Run(ctx, c.rdb, []string{
		schedulerAccountEpochKey(id),
		schedulerAccountFenceKey(id),
		schedulerAccountTombstoneKey(id),
		schedulerAccountKey(id),
		schedulerAccountMetaKey(id),
		schedulerAccountVersionKey(id),
	}, strconv.FormatInt(epoch, 10), fullPayload, metaPayload, schedulerAccountVersion(*account)).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (c *schedulerCache) CompleteAccountDeletion(ctx context.Context, accountID, epoch int64) (bool, error) {
	if accountID <= 0 || epoch <= 0 {
		return false, nil
	}
	id := strconv.FormatInt(accountID, 10)
	result, err := completeAccountDeletionScript.Run(ctx, c.rdb, []string{
		schedulerAccountEpochKey(id),
		schedulerAccountFenceKey(id),
		schedulerAccountTombstoneKey(id),
		schedulerAccountKey(id),
		schedulerAccountMetaKey(id),
		schedulerAccountVersionKey(id),
	}, strconv.FormatInt(epoch, 10)).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (c *schedulerCache) ReconcileAccountMutations(ctx context.Context, accounts map[int64]*service.Account, epochs map[int64]int64) (map[int64]bool, error) {
	if len(epochs) == 0 {
		return map[int64]bool{}, nil
	}
	pipe := c.rdb.Pipeline()
	commands := make(map[int64]*redis.Cmd, len(epochs))
	for accountID, epoch := range epochs {
		if accountID <= 0 || epoch <= 0 {
			continue
		}
		id := strconv.FormatInt(accountID, 10)
		keys := []string{
			schedulerAccountEpochKey(id),
			schedulerAccountFenceKey(id),
			schedulerAccountTombstoneKey(id),
			schedulerAccountKey(id),
			schedulerAccountMetaKey(id),
			schedulerAccountVersionKey(id),
		}
		if account := accounts[accountID]; account != nil {
			fullPayload, metaPayload, err := encodeSchedulerAccount(*account)
			if err != nil {
				return nil, err
			}
			commands[accountID] = pipe.Eval(ctx, publishAccountMutationLua, keys,
				strconv.FormatInt(epoch, 10), fullPayload, metaPayload, schedulerAccountVersion(*account))
		} else {
			commands[accountID] = pipe.Eval(ctx, completeAccountDeletionLua, keys,
				strconv.FormatInt(epoch, 10))
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	results := make(map[int64]bool, len(commands))
	for accountID, command := range commands {
		value, err := command.Int64()
		if err != nil {
			return results, err
		}
		results[accountID] = value == 1
	}
	return results, nil
}

func (c *schedulerCache) DeleteAccount(ctx context.Context, accountID int64) error {
	if accountID <= 0 {
		return nil
	}
	id := strconv.FormatInt(accountID, 10)
	return c.rdb.Del(ctx, schedulerAccountKey(id), schedulerAccountMetaKey(id), schedulerAccountVersionKey(id)).Err()
}

func (c *schedulerCache) UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	if len(updates) == 0 {
		return nil
	}

	keys := make([]string, 0, len(updates))
	ids := make([]int64, 0, len(updates))
	for id := range updates {
		keys = append(keys, schedulerAccountKey(strconv.FormatInt(id, 10)))
		ids = append(ids, id)
	}

	values, err := c.mgetChunked(ctx, keys)
	if err != nil {
		return err
	}

	pipe := c.rdb.Pipeline()
	for i, val := range values {
		if val == nil {
			continue
		}
		account, err := decodeCachedAccount(val)
		if err != nil {
			return err
		}
		candidate := updates[ids[i]]
		if account.LastUsedAt != nil && !candidate.After(*account.LastUsedAt) {
			continue
		}
		account.LastUsedAt = ptrTime(candidate)
		updated, metaPayload, err := encodeSchedulerAccount(*account)
		if err != nil {
			return err
		}
		id := strconv.FormatInt(ids[i], 10)
		pipe.Eval(ctx, compareAndSetAccountLua, []string{
			schedulerAccountFenceKey(id),
			schedulerAccountTombstoneKey(id),
			keys[i],
			schedulerAccountMetaKey(id),
		}, val, updated, metaPayload)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (c *schedulerCache) TryLockBucket(ctx context.Context, bucket service.SchedulerBucket, ttl time.Duration) (bool, error) {
	key := schedulerBucketKey(schedulerLockPrefix, bucket)
	return c.rdb.SetNX(ctx, key, time.Now().UnixNano(), ttl).Result()
}

func (c *schedulerCache) UnlockBucket(ctx context.Context, bucket service.SchedulerBucket) error {
	key := schedulerBucketKey(schedulerLockPrefix, bucket)
	return c.rdb.Del(ctx, key).Err()
}

func (c *schedulerCache) ListBuckets(ctx context.Context) ([]service.SchedulerBucket, error) {
	raw, err := c.rdb.SMembers(ctx, schedulerBucketSetKey).Result()
	if err != nil {
		return nil, err
	}
	out := make([]service.SchedulerBucket, 0, len(raw))
	for _, entry := range raw {
		bucket, ok := service.ParseSchedulerBucket(entry)
		if !ok {
			continue
		}
		out = append(out, bucket)
	}
	return out, nil
}

func (c *schedulerCache) GetOutboxWatermark(ctx context.Context) (int64, error) {
	val, err := c.rdb.Get(ctx, schedulerOutboxWatermarkKey).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (c *schedulerCache) SetOutboxWatermark(ctx context.Context, id int64) error {
	return c.rdb.Set(ctx, schedulerOutboxWatermarkKey, strconv.FormatInt(id, 10), 0).Err()
}

func schedulerBucketKey(prefix string, bucket service.SchedulerBucket) string {
	return fmt.Sprintf("%s%d:%s:%s", prefix, bucket.GroupID, bucket.Platform, bucket.Mode)
}

func schedulerSnapshotKey(bucket service.SchedulerBucket, version string) string {
	return fmt.Sprintf("%s%d:%s:%s:v%s", schedulerSnapshotPrefix, bucket.GroupID, bucket.Platform, bucket.Mode, version)
}

func schedulerAccountKey(id string) string {
	return schedulerAccountPrefix + id
}

func schedulerAccountMetaKey(id string) string {
	return schedulerAccountMetaPrefix + id
}

func schedulerAccountEpochKey(id string) string {
	return schedulerAccountEpochPrefix + id
}

func schedulerAccountVersionKey(id string) string {
	return schedulerAccountVersionPrefix + id
}

func schedulerAccountFenceKey(id string) string {
	return schedulerAccountFencePrefix + id
}

func schedulerAccountTombstoneKey(id string) string {
	return schedulerAccountTombPrefix + id
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func decodeCachedAccount(val any) (*service.Account, error) {
	var payload []byte
	switch raw := val.(type) {
	case string:
		payload = []byte(raw)
	case []byte:
		payload = raw
	default:
		return nil, fmt.Errorf("unexpected account cache type: %T", val)
	}
	var account service.Account
	if err := json.Unmarshal(payload, &account); err != nil {
		return nil, err
	}
	return &account, nil
}

func (c *schedulerCache) writeAccounts(ctx context.Context, accounts []service.Account) error {
	if len(accounts) == 0 {
		return nil
	}

	type queuedAccountWrite struct {
		account     service.Account
		fullPayload []byte
		metaPayload []byte
		command     *redis.Cmd
	}

	pipe := c.rdb.Pipeline()
	queued := make([]queuedAccountWrite, 0, c.writeChunkSize)
	flush := func() error {
		if len(queued) == 0 {
			return nil
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
		for _, write := range queued {
			result, err := write.command.Int64()
			if err != nil {
				return err
			}
			switch result {
			case 2:
				if err := c.upgradeLegacyAccountVersion(ctx, write.account, write.fullPayload, write.metaPayload); err != nil {
					return err
				}
			case 3:
				if err := c.mergeRestrictiveQuotaState(ctx, write.account); err != nil {
					return err
				}
			}
		}
		pipe = c.rdb.Pipeline()
		queued = queued[:0]
		return nil
	}

	for _, account := range accounts {
		fullPayload, metaPayload, err := encodeSchedulerAccount(account)
		if err != nil {
			return err
		}

		id := strconv.FormatInt(account.ID, 10)
		command := pipe.Eval(ctx, guardedSetAccountLua, []string{
			schedulerAccountFenceKey(id),
			schedulerAccountEpochKey(id),
			schedulerAccountTombstoneKey(id),
			schedulerAccountKey(id),
			schedulerAccountMetaKey(id),
			schedulerAccountVersionKey(id),
		}, fullPayload, metaPayload, schedulerAccountVersion(account), strconv.Itoa(schedulerAccountQuotaRestrictionMask(account)))
		queued = append(queued, queuedAccountWrite{
			account:     account,
			fullPayload: fullPayload,
			metaPayload: metaPayload,
			command:     command,
		})
		if len(queued) >= c.writeChunkSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}

	return flush()
}

// upgradeLegacyAccountVersion safely adopts a full value from an older v2
// binary or partial cache state that predates sched:v2:acc:version. The
// optimistic transaction compares the candidate with cached Account.UpdatedAt
// and watches every fence/value key, so a concurrent mutation or write can only
// make this repair retry or no-op; it cannot turn an old snapshot into a
// successful overwrite.
func (c *schedulerCache) upgradeLegacyAccountVersion(
	ctx context.Context,
	account service.Account,
	fullPayload []byte,
	metaPayload []byte,
) error {
	id := strconv.FormatInt(account.ID, 10)
	fenceKey := schedulerAccountFenceKey(id)
	epochKey := schedulerAccountEpochKey(id)
	tombstoneKey := schedulerAccountTombstoneKey(id)
	fullKey := schedulerAccountKey(id)
	metaKey := schedulerAccountMetaKey(id)
	versionKey := schedulerAccountVersionKey(id)
	candidateVersion := schedulerAccountVersion(account)
	quotaRestrictionMask := schedulerAccountQuotaRestrictionMask(account)
	watchKeys := []string{fenceKey, epochKey, tombstoneKey, fullKey, versionKey}

	for range 3 {
		err := c.rdb.Watch(ctx, func(tx *redis.Tx) error {
			values, err := tx.MGet(ctx, fenceKey, epochKey, tombstoneKey, fullKey, versionKey).Result()
			if err != nil {
				return err
			}
			if values[0] != nil || values[2] != nil || values[3] == nil {
				return nil
			}

			versionToStore := candidateVersion
			fullToStore := fullPayload
			metaToStore := metaPayload
			if currentVersion, ok := values[4].(string); ok {
				if candidateVersion < currentVersion || (values[1] != nil && candidateVersion == currentVersion) {
					if quotaRestrictionMask == 0 {
						return nil
					}
					cached, err := decodeCachedAccount(values[3])
					if err != nil {
						return err
					}
					if !mergeSchedulerQuotaRestrictions(cached, account, quotaRestrictionMask) {
						return nil
					}
					fullToStore, metaToStore, err = encodeSchedulerAccount(*cached)
					if err != nil {
						return err
					}
					versionToStore = currentVersion
				}
			} else {
				cached, err := decodeCachedAccount(values[3])
				if err != nil {
					return err
				}
				// Epoch-bearing legacy values require a strictly later DB source
				// revision. Equality cannot distinguish a fresh rebuild from a
				// snapshot that was loaded before a group-only mutation.
				cachedVersion := schedulerAccountVersion(*cached)
				if values[1] != nil && candidateVersion <= cachedVersion {
					if quotaRestrictionMask == 0 {
						return nil
					}
					if !mergeSchedulerQuotaRestrictions(cached, account, quotaRestrictionMask) {
						return nil
					}
					fullToStore, metaToStore, err = encodeSchedulerAccount(*cached)
					if err != nil {
						return err
					}
					versionToStore = cachedVersion
				}
			}

			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, fullKey, fullToStore, 0)
				pipe.Set(ctx, metaKey, metaToStore, 0)
				pipe.Set(ctx, versionKey, versionToStore, 0)
				return nil
			})
			return err
		}, watchKeys...)
		if err == redis.TxFailedErr {
			continue
		}
		return err
	}

	// Repeated contention leaves the newer/unknown value intact. Ordinary cache
	// refreshes are best-effort; failing closed is safer than a blind final write.
	return nil
}

// mergeRestrictiveQuotaState applies only quota enforcement from a
// non-comparable ordinary DB revision. Keeping identity, status, credentials,
// groups, and routing fields from the current version prevents a stale expiring
// quota snapshot from resurrecting an older lease when its window resets.
func (c *schedulerCache) mergeRestrictiveQuotaState(ctx context.Context, candidate service.Account) error {
	mask := schedulerAccountQuotaRestrictionMask(candidate)
	if mask == 0 {
		return nil
	}
	id := strconv.FormatInt(candidate.ID, 10)
	fenceKey := schedulerAccountFenceKey(id)
	tombstoneKey := schedulerAccountTombstoneKey(id)
	fullKey := schedulerAccountKey(id)
	metaKey := schedulerAccountMetaKey(id)
	versionKey := schedulerAccountVersionKey(id)
	watchKeys := []string{fenceKey, tombstoneKey, fullKey, versionKey}

	for range 3 {
		err := c.rdb.Watch(ctx, func(tx *redis.Tx) error {
			values, err := tx.MGet(ctx, fenceKey, tombstoneKey, fullKey, versionKey).Result()
			if err != nil {
				return err
			}
			if values[0] != nil {
				return service.ErrSchedulerAccountMutationInProgress
			}
			if values[1] != nil || values[2] == nil || values[3] == nil {
				return nil
			}
			current, err := decodeCachedAccount(values[2])
			if err != nil {
				return err
			}
			if !mergeSchedulerQuotaRestrictions(current, candidate, mask) {
				return nil
			}
			fullPayload, metaPayload, err := encodeSchedulerAccount(*current)
			if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, fullKey, fullPayload, 0)
				pipe.Set(ctx, metaKey, metaPayload, 0)
				return nil
			})
			return err
		}, watchKeys...)
		if err == redis.TxFailedErr {
			continue
		}
		return err
	}

	// Unlike a best-effort ordinary rebuild, a quota crossing is an enforcement
	// event. Surface sustained contention so the outbox watermark is not advanced
	// and the event can be retried.
	return redis.TxFailedErr
}

func encodeSchedulerAccount(account service.Account) ([]byte, []byte, error) {
	fullPayload, err := json.Marshal(account)
	if err != nil {
		return nil, nil, err
	}
	metaPayload, err := json.Marshal(buildSchedulerMetadataAccount(account))
	if err != nil {
		return nil, nil, err
	}
	return fullPayload, metaPayload, nil
}

// schedulerAccountVersion returns a fixed-width, lexicographically ordered
// source revision. Account.UpdatedAt is populated by every authoritative
// repository read; keeping the comparison in Redis prevents an ordinary
// rebuild that loaded before a fenced mutation from overwriting its publish.
// The zero time remains ordered below every persisted account timestamp and is
// useful for fail-closed behavior in lightweight tests and malformed payloads.
func schedulerAccountVersion(account service.Account) string {
	return account.UpdatedAt.UTC().Format("20060102150405.000000000")
}

const (
	schedulerQuotaRestrictionTotal = 1 << iota
	schedulerQuotaRestrictionDaily
	schedulerQuotaRestrictionWeekly
)

func schedulerAccountQuotaRestrictionMask(account service.Account) int {
	if !account.IsAPIKeyOrBedrock() {
		return 0
	}
	mask := 0
	if limit := account.GetQuotaLimit(); limit > 0 && account.GetQuotaUsed() >= limit {
		mask |= schedulerQuotaRestrictionTotal
	}
	if limit := account.GetQuotaDailyLimit(); limit > 0 && account.GetQuotaDailyUsed() >= limit && !account.IsDailyQuotaPeriodExpired() {
		mask |= schedulerQuotaRestrictionDaily
	}
	if limit := account.GetQuotaWeeklyLimit(); limit > 0 && account.GetQuotaWeeklyUsed() >= limit && !account.IsWeeklyQuotaPeriodExpired() {
		mask |= schedulerQuotaRestrictionWeekly
	}
	return mask
}

func mergeSchedulerQuotaRestrictions(current *service.Account, candidate service.Account, mask int) bool {
	if current == nil || mask == 0 || !current.IsAPIKeyOrBedrock() {
		return false
	}
	changed := false
	if mask&schedulerQuotaRestrictionTotal != 0 {
		mergedExtra := make(map[string]any, len(current.Extra)+2)
		for key, value := range current.Extra {
			mergedExtra[key] = value
		}
		for _, key := range []string{"quota_limit", "quota_used"} {
			if value, ok := candidate.Extra[key]; ok {
				mergedExtra[key] = value
			} else {
				delete(mergedExtra, key)
			}
		}
		current.Extra = mergedExtra
		changed = true
	}
	if mask&(schedulerQuotaRestrictionDaily|schedulerQuotaRestrictionWeekly) != 0 {
		until := schedulerQuotaWindowRestrictionUntil(candidate, mask, time.Now())
		if !until.IsZero() && (current.TempUnschedulableUntil == nil || current.TempUnschedulableUntil.Before(until)) {
			current.TempUnschedulableUntil = ptrTime(until)
			current.TempUnschedulableReason = "quota_window_version_fence"
			changed = true
		}
	}
	return changed
}

func schedulerQuotaWindowRestrictionUntil(account service.Account, mask int, now time.Time) time.Time {
	var until time.Time
	consider := func(candidate time.Time, fallback time.Duration) {
		if candidate.IsZero() || !candidate.After(now) {
			candidate = now.Add(fallback)
		}
		if candidate.After(until) {
			until = candidate
		}
	}
	if mask&schedulerQuotaRestrictionDaily != 0 {
		if account.GetQuotaDailyResetMode() == "fixed" {
			consider(schedulerAccountExtraTime(account.Extra, "quota_daily_reset_at"), 26*time.Hour)
		} else {
			consider(schedulerAccountExtraTime(account.Extra, "quota_daily_start").Add(24*time.Hour), 24*time.Hour)
		}
	}
	if mask&schedulerQuotaRestrictionWeekly != 0 {
		if account.GetQuotaWeeklyResetMode() == "fixed" {
			consider(schedulerAccountExtraTime(account.Extra, "quota_weekly_reset_at"), 8*24*time.Hour)
		} else {
			consider(schedulerAccountExtraTime(account.Extra, "quota_weekly_start").Add(7*24*time.Hour), 7*24*time.Hour)
		}
	}
	return until
}

func schedulerAccountExtraTime(extra map[string]any, key string) time.Time {
	raw, _ := extra[key].(string)
	parsed, _ := time.Parse(time.RFC3339Nano, raw)
	return parsed
}

func uniquePositiveSchedulerAccountIDs(values []int64) []int64 {
	result := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (c *schedulerCache) mgetChunked(ctx context.Context, keys []string) ([]any, error) {
	if len(keys) == 0 {
		return []any{}, nil
	}

	out := make([]any, 0, len(keys))
	chunkSize := c.mgetChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultSchedulerSnapshotMGetChunkSize
	}
	for start := 0; start < len(keys); start += chunkSize {
		end := start + chunkSize
		if end > len(keys) {
			end = len(keys)
		}
		part, err := c.rdb.MGet(ctx, keys[start:end]...).Result()
		if err != nil {
			return nil, err
		}
		out = append(out, part...)
	}
	return out, nil
}

func buildSchedulerMetadataAccount(account service.Account) service.Account {
	return service.Account{
		ID:                      account.ID,
		Name:                    account.Name,
		Platform:                account.Platform,
		Type:                    account.Type,
		Concurrency:             account.Concurrency,
		LoadFactor:              account.LoadFactor,
		Priority:                account.Priority,
		RateMultiplier:          account.RateMultiplier,
		Status:                  account.Status,
		LastUsedAt:              account.LastUsedAt,
		ExpiresAt:               account.ExpiresAt,
		AutoPauseOnExpired:      account.AutoPauseOnExpired,
		Schedulable:             account.Schedulable,
		RateLimitedAt:           account.RateLimitedAt,
		RateLimitResetAt:        account.RateLimitResetAt,
		OverloadUntil:           account.OverloadUntil,
		TempUnschedulableUntil:  account.TempUnschedulableUntil,
		TempUnschedulableReason: account.TempUnschedulableReason,
		SessionWindowStart:      account.SessionWindowStart,
		SessionWindowEnd:        account.SessionWindowEnd,
		SessionWindowStatus:     account.SessionWindowStatus,
		AccountGroups:           filterSchedulerAccountGroups(account.AccountGroups),
		GroupIDs:                filterSchedulerGroupIDs(account.GroupIDs, account.AccountGroups),
		Credentials:             filterSchedulerCredentials(account.Credentials),
		Extra:                   filterSchedulerExtra(account.Extra),
	}
}

func filterSchedulerAccountGroups(accountGroups []service.AccountGroup) []service.AccountGroup {
	if len(accountGroups) == 0 {
		return nil
	}

	filtered := make([]service.AccountGroup, 0, len(accountGroups))
	for _, ag := range accountGroups {
		if ag.GroupID <= 0 {
			continue
		}
		filtered = append(filtered, service.AccountGroup{
			AccountID: ag.AccountID,
			GroupID:   ag.GroupID,
			Priority:  ag.Priority,
			CreatedAt: ag.CreatedAt,
		})
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func filterSchedulerGroupIDs(groupIDs []int64, accountGroups []service.AccountGroup) []int64 {
	if len(groupIDs) == 0 && len(accountGroups) == 0 {
		return nil
	}

	seen := make(map[int64]struct{}, len(groupIDs)+len(accountGroups))
	filtered := make([]int64, 0, len(groupIDs)+len(accountGroups))
	for _, id := range groupIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		filtered = append(filtered, id)
	}
	for _, ag := range accountGroups {
		if ag.GroupID <= 0 {
			continue
		}
		if _, ok := seen[ag.GroupID]; ok {
			continue
		}
		seen[ag.GroupID] = struct{}{}
		filtered = append(filtered, ag.GroupID)
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func filterSchedulerCredentials(credentials map[string]any) map[string]any {
	if len(credentials) == 0 {
		return nil
	}
	keys := []string{"model_mapping", "api_key", "base_url", "project_id", "oauth_type"}
	filtered := make(map[string]any)
	for _, key := range keys {
		if value, ok := credentials[key]; ok && value != nil {
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func filterSchedulerExtra(extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return nil
	}
	keys := []string{
		"mixed_scheduling",
		"window_cost_limit",
		"window_cost_sticky_reserve",
		"max_sessions",
		"session_idle_timeout_minutes",
		"openai_oauth_responses_websockets_v2_enabled",
		"openai_oauth_responses_websockets_v2_mode",
		"openai_apikey_responses_websockets_v2_enabled",
		"openai_apikey_responses_websockets_v2_mode",
		"responses_websockets_v2_enabled",
		"openai_ws_enabled",
		"openai_ws_force_http",
		"aether_ws",
		"openai_responses_mode",
		"openai_responses_supported",
		"cafecode_identity_headers_enabled",
		"codex_5h_used_percent",
		"codex_7d_used_percent",
		"codex_5h_reset_at",
		"codex_7d_reset_at",
		"codex_5h_reset_after_seconds",
		"codex_7d_reset_after_seconds",
		"codex_usage_updated_at",
		"auto_pause_5h_threshold",
		"auto_pause_7d_threshold",
		"auto_pause_5h_disabled",
		"auto_pause_7d_disabled",
		"model_rate_limits",
	}
	filtered := make(map[string]any)
	for _, key := range keys {
		if value, ok := extra[key]; ok && value != nil {
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}
