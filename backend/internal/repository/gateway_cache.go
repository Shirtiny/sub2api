package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const stickySessionPrefix = "sticky_session:"
const openAIWSMigrationPrefix = "openai_ws_migration:"
const openAIWSMigrationExcludedFieldPrefix = "excluded:"

var admitOpenAIWSReconnectMigrationScript = redis.NewScript(`
local migration_key = KEYS[1]
local sticky_key = KEYS[2]
local redis_time = redis.call('TIME')
local now_ms = tonumber(redis_time[1]) * 1000 + math.floor(tonumber(redis_time[2]) / 1000)
local window_ms = tonumber(ARGV[2])
local exclusion_ms = tonumber(ARGV[3])
local account_id = ARGV[4]
local control_id = ARGV[5]
local expected_generation = tonumber(ARGV[6])
local max_migrations = tonumber(ARGV[7])
local middle_route_disposition = ARGV[8]
if middle_route_disposition ~= 'retain' and middle_route_disposition ~= 'exclude' then
  return redis.error_reply('openai ws migration middle route disposition is invalid')
end

local window_started_at_ms = tonumber(redis.call('HGET', migration_key, 'window_started_at_ms'))
if not window_started_at_ms or now_ms - window_started_at_ms >= window_ms then
  redis.call('DEL', migration_key)
  window_started_at_ms = now_ms
end

local control_field = 'control:' .. control_id
local existing_control = redis.call('HGET', migration_key, control_field)
if existing_control then
	local existing_expected, existing_account, existing_disposition, existing_count, existing_generation, existing_exclusion_until =
	  string.match(existing_control, '^(%d+):(%d+):([a-z]+):(%d+):(%d+):(%d+)$')
	if not existing_expected or tonumber(existing_expected) ~= expected_generation or
	    existing_account ~= account_id or existing_disposition ~= middle_route_disposition then
	  return redis.error_reply('openai ws migration control id was reused')
	end
	return {1, tonumber(existing_count), tonumber(existing_generation), 1, window_started_at_ms, 0, now_ms, tonumber(existing_exclusion_until)}
end

local migration_count = tonumber(redis.call('HGET', migration_key, 'migration_count')) or 0
local current_generation = tonumber(redis.call('HGET', migration_key, 'binding_generation')) or 1
if current_generation ~= expected_generation then
  return {0, migration_count, current_generation, 0, window_started_at_ms, 2, now_ms, 0}
end
if migration_count >= max_migrations then
  return {0, migration_count, current_generation, 0, window_started_at_ms, 1, now_ms, 0}
end

local next_count = migration_count + 1
local next_generation = current_generation + 1
local exclusion_until = 0
if middle_route_disposition == 'exclude' then
  exclusion_until = now_ms + exclusion_ms
end
redis.call('HSET', migration_key,
  'window_started_at_ms', window_started_at_ms,
  'migration_count', next_count,
  'binding_generation', next_generation,
  control_field, expected_generation .. ':' .. account_id .. ':' .. middle_route_disposition .. ':' .. next_count .. ':' .. next_generation .. ':' .. exclusion_until)
if middle_route_disposition == 'exclude' then
  redis.call('HSET', migration_key, 'excluded:' .. account_id, exclusion_until)
  redis.call('DEL', sticky_key)
end
local remaining_window_ms = window_ms - (now_ms - window_started_at_ms)
if remaining_window_ms < 1 then
  remaining_window_ms = 1
end
redis.call('PEXPIRE', migration_key, remaining_window_ms)
return {1, next_count, next_generation, 0, window_started_at_ms, 0, now_ms, exclusion_until}
`)

type gatewayCache struct {
	rdb *redis.Client
}

var _ service.OpenAIWSMigrationCache = (*gatewayCache)(nil)

func NewGatewayCache(rdb *redis.Client) service.GatewayCache {
	return &gatewayCache{rdb: rdb}
}

// buildSessionKey 构建 session key，包含 groupID 实现分组隔离
// 格式: sticky_session:{groupID}:{sessionHash}
func buildSessionKey(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", stickySessionPrefix, groupID, sessionHash)
}

func buildOpenAIWSMigrationKey(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", openAIWSMigrationPrefix, groupID, sessionHash)
}

func (c *gatewayCache) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Get(ctx, key).Int64()
}

func (c *gatewayCache) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Set(ctx, key, accountID, ttl).Err()
}

func (c *gatewayCache) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Expire(ctx, key, ttl).Err()
}

// DeleteSessionAccountID 删除粘性会话与账号的绑定关系。
// 当检测到绑定的账号不可用（如状态错误、禁用、不可调度等）时调用，
// 以便下次请求能够重新选择可用账号。
//
// DeleteSessionAccountID removes the sticky session binding for the given session.
// Called when the bound account becomes unavailable (e.g., error status, disabled,
// or unschedulable), allowing subsequent requests to select a new available account.
func (c *gatewayCache) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Del(ctx, key).Err()
}

func (c *gatewayCache) LoadOpenAIWSMigrationState(
	ctx context.Context,
	groupID int64,
	sessionHash string,
) (service.OpenAIWSMigrationCacheState, error) {
	pipe := c.rdb.Pipeline()
	valuesCmd := pipe.HGetAll(ctx, buildOpenAIWSMigrationKey(groupID, sessionHash))
	timeCmd := pipe.Time(ctx)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return service.OpenAIWSMigrationCacheState{}, err
	}
	values := valuesCmd.Val()
	redisNow := timeCmd.Val()
	state := service.OpenAIWSMigrationCacheState{
		ExcludedUntilUnixMilli: make(map[int64]int64),
		ObservedAtUnixMilli:    redisNow.UnixMilli(),
	}
	for field, value := range values {
		switch field {
		case "window_started_at_ms":
			parsed, parseErr := strconv.ParseInt(value, 10, 64)
			if parseErr != nil || parsed <= 0 {
				return service.OpenAIWSMigrationCacheState{}, fmt.Errorf("invalid openai ws migration window state")
			}
			state.WindowStartedAtUnixMilli = parsed
		case "migration_count":
			parsed, parseErr := strconv.Atoi(value)
			if parseErr != nil || parsed < 0 {
				return service.OpenAIWSMigrationCacheState{}, fmt.Errorf("invalid openai ws migration count state")
			}
			state.MigrationCount = parsed
		case "binding_generation":
			parsed, parseErr := strconv.ParseUint(value, 10, 64)
			if parseErr != nil || parsed == 0 {
				return service.OpenAIWSMigrationCacheState{}, fmt.Errorf("invalid openai ws migration generation state")
			}
			state.BindingGeneration = parsed
		default:
			if !strings.HasPrefix(field, openAIWSMigrationExcludedFieldPrefix) {
				continue
			}
			accountID, accountErr := strconv.ParseInt(strings.TrimPrefix(field, openAIWSMigrationExcludedFieldPrefix), 10, 64)
			until, untilErr := strconv.ParseInt(value, 10, 64)
			if accountErr != nil || untilErr != nil || accountID <= 0 || until <= 0 {
				return service.OpenAIWSMigrationCacheState{}, fmt.Errorf("invalid openai ws migration exclusion state")
			}
			state.ExcludedUntilUnixMilli[accountID] = until
		}
	}
	if len(values) > 0 && (state.WindowStartedAtUnixMilli == 0 || state.BindingGeneration == 0) {
		return service.OpenAIWSMigrationCacheState{}, fmt.Errorf("incomplete openai ws migration state")
	}
	return state, nil
}

func (c *gatewayCache) AdmitOpenAIWSReconnectMigration(
	ctx context.Context,
	groupID int64,
	sessionHash string,
	accountID int64,
	controlID string,
	bindingGeneration uint64,
	middleRouteDisposition service.OpenAIWSMiddleRouteDisposition,
	nowUnixMilli int64,
	window time.Duration,
	exclusionTTL time.Duration,
	maxMigrations int,
) (service.OpenAIWSMigrationCacheDecision, error) {
	result, err := admitOpenAIWSReconnectMigrationScript.Run(
		ctx,
		c.rdb,
		[]string{
			buildOpenAIWSMigrationKey(groupID, sessionHash),
			buildSessionKey(groupID, sessionHash),
		},
		nowUnixMilli,
		window.Milliseconds(),
		exclusionTTL.Milliseconds(),
		accountID,
		controlID,
		bindingGeneration,
		maxMigrations,
		string(middleRouteDisposition),
	).Int64Slice()
	if err != nil {
		return service.OpenAIWSMigrationCacheDecision{}, err
	}
	if len(result) != 8 {
		return service.OpenAIWSMigrationCacheDecision{}, fmt.Errorf("unexpected openai ws migration script result length: %d", len(result))
	}
	if result[5] == 2 {
		return service.OpenAIWSMigrationCacheDecision{}, fmt.Errorf(
			"openai ws migration binding generation mismatch: expected %d, current %d",
			bindingGeneration,
			result[2],
		)
	}
	decision := service.OpenAIWSMigrationCacheDecision{
		State: service.OpenAIWSMigrationCacheState{
			WindowStartedAtUnixMilli: result[4],
			ObservedAtUnixMilli:      result[6],
			MigrationCount:           int(result[1]),
			BindingGeneration:        uint64(result[2]),
			ExcludedUntilUnixMilli:   make(map[int64]int64),
		},
		Admitted:   result[0] == 1,
		Idempotent: result[3] == 1,
		Exhausted:  result[5] == 1,
	}
	if decision.Admitted && middleRouteDisposition == service.OpenAIWSMiddleRouteDispositionExclude {
		if result[7] <= 0 {
			return service.OpenAIWSMigrationCacheDecision{}, fmt.Errorf("openai ws migration exclusion deadline is missing")
		}
		decision.State.ExcludedUntilUnixMilli[accountID] = result[7]
	}
	return decision, nil
}
