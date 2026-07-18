package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	// Flagged hashes live in a sorted set scored by expiry unix seconds. The
	// legacy key held the same members in a plain set with no expiry, so it is
	// a different Redis type and can only be dropped, never read.
	contentModerationFlaggedHashKey           = "content_moderation:flagged_hashes:v2"
	contentModerationSideEffectDedupKeyPrefix = "content_moderation:side_effect_dedup:v2:"
	contentModerationLegacySideEffectDedupKey = "content_moderation:side_effect_dedup:v1"
	contentModerationLegacyFlaggedHashKey     = "content_moderation:flagged_hashes"
)

var (
	reserveContentModerationSideEffectScript = redis.NewScript(`
local current = redis.call('HGET', KEYS[1], ARGV[1])
if current then
  local expires_at = tonumber(string.match(current, ':(%d+)$'))
  if expires_at and expires_at > tonumber(ARGV[2]) then
    return 0
  end
end
redis.call('HSET', KEYS[1], ARGV[1], ARGV[3])
local current_ttl = redis.call('PTTL', KEYS[1])
if current_ttl < tonumber(ARGV[4]) then
  redis.call('PEXPIRE', KEYS[1], ARGV[4])
end
return 1
`)
	finalizeContentModerationSideEffectScript = redis.NewScript(`
local current = redis.call('HGET', KEYS[1], ARGV[1])
if not current or string.sub(current, 1, string.len(ARGV[2])) ~= ARGV[2] then
  return 0
end
redis.call('HSET', KEYS[1], ARGV[1], ARGV[3])
local current_ttl = redis.call('PTTL', KEYS[1])
if current_ttl < tonumber(ARGV[4]) then
  redis.call('PEXPIRE', KEYS[1], ARGV[4])
end
return 1
`)
	releaseContentModerationSideEffectScript = redis.NewScript(`
local current = redis.call('HGET', KEYS[1], ARGV[1])
if not current or string.sub(current, 1, string.len(ARGV[2])) ~= ARGV[2] then
  return 0
end
return redis.call('HDEL', KEYS[1], ARGV[1])
`)
	deleteContentModerationHashScript = redis.NewScript(`
local deleted = redis.call('ZREM', KEYS[1], ARGV[1])
redis.call('DEL', KEYS[2])
return deleted
`)
	clearContentModerationHashesScript = redis.NewScript(`
local members = redis.call('ZRANGE', KEYS[1], 0, -1)
for _, member in ipairs(members) do
  redis.call('DEL', ARGV[1] .. member)
end
local count = redis.call('ZCARD', KEYS[1])
redis.call('DEL', KEYS[1], KEYS[2], KEYS[3])
return count
`)
)

type contentModerationHashCache struct {
	rdb *redis.Client
}

func NewContentModerationHashCache(rdb *redis.Client) service.ContentModerationHashCache {
	return &contentModerationHashCache{rdb: rdb}
}

func (c *contentModerationHashCache) RecordFlaggedInputHash(ctx context.Context, inputHash string, ttl time.Duration) error {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" || ttl <= 0 {
		return nil
	}
	now := time.Now()
	pipe := c.rdb.TxPipeline()
	pipe.ZAdd(ctx, contentModerationFlaggedHashKey, redis.Z{
		Score:  float64(now.Add(ttl).Unix()),
		Member: inputHash,
	})
	pipe.ZRemRangeByScore(ctx, contentModerationFlaggedHashKey, "-inf", strconv.FormatInt(now.Unix(), 10))
	_, err := pipe.Exec(ctx)
	return err
}

func (c *contentModerationHashCache) HasFlaggedInputHash(ctx context.Context, inputHash string) (bool, error) {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" {
		return false, nil
	}
	expiresAt, err := c.rdb.ZScore(ctx, contentModerationFlaggedHashKey, inputHash).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return int64(expiresAt) > time.Now().Unix(), nil
}

// ReserveFlaggedInputSideEffect creates a short-lived, token-owned reservation.
// Callers must finalize it after the effect succeeds or release it on failure.
func (c *contentModerationHashCache) ReserveFlaggedInputSideEffect(ctx context.Context, subjectKey string, effectType string, inputHash string, token string, reservationTTL time.Duration, retentionTTL time.Duration) (bool, error) {
	subjectKey = strings.TrimSpace(subjectKey)
	effectType = strings.TrimSpace(effectType)
	inputHash = strings.TrimSpace(inputHash)
	token = strings.TrimSpace(token)
	if c == nil || c.rdb == nil || subjectKey == "" || effectType == "" || inputHash == "" || token == "" || reservationTTL <= 0 || retentionTTL <= 0 {
		return false, nil
	}

	now := time.Now()
	reservedValue := contentModerationSideEffectReservationValue(token, now.Add(reservationTTL))
	keyTTL := max(retentionTTL, reservationTTL)
	reserved, err := reserveContentModerationSideEffectScript.Run(
		ctx,
		c.rdb,
		[]string{contentModerationSideEffectKey(inputHash)},
		contentModerationSideEffectField(subjectKey, effectType),
		now.UnixMilli(),
		reservedValue,
		keyTTL.Milliseconds(),
	).Int64()
	if err != nil {
		return false, err
	}
	return reserved > 0, nil
}

func (c *contentModerationHashCache) FinalizeFlaggedInputSideEffect(ctx context.Context, subjectKey string, effectType string, inputHash string, token string, retentionTTL time.Duration) (bool, error) {
	subjectKey = strings.TrimSpace(subjectKey)
	effectType = strings.TrimSpace(effectType)
	inputHash = strings.TrimSpace(inputHash)
	token = strings.TrimSpace(token)
	if c == nil || c.rdb == nil || subjectKey == "" || effectType == "" || inputHash == "" || token == "" || retentionTTL <= 0 {
		return false, nil
	}
	now := time.Now()
	finalized, err := finalizeContentModerationSideEffectScript.Run(
		ctx,
		c.rdb,
		[]string{contentModerationSideEffectKey(inputHash)},
		contentModerationSideEffectField(subjectKey, effectType),
		contentModerationSideEffectReservationValue(token, time.Time{}),
		contentModerationSideEffectFinalValue(now.Add(retentionTTL)),
		retentionTTL.Milliseconds(),
	).Int64()
	if err != nil {
		return false, err
	}
	return finalized > 0, nil
}

func (c *contentModerationHashCache) ReleaseFlaggedInputSideEffect(ctx context.Context, subjectKey string, effectType string, inputHash string, token string) (bool, error) {
	subjectKey = strings.TrimSpace(subjectKey)
	effectType = strings.TrimSpace(effectType)
	inputHash = strings.TrimSpace(inputHash)
	token = strings.TrimSpace(token)
	if c == nil || c.rdb == nil || subjectKey == "" || effectType == "" || inputHash == "" || token == "" {
		return false, nil
	}
	released, err := releaseContentModerationSideEffectScript.Run(
		ctx,
		c.rdb,
		[]string{contentModerationSideEffectKey(inputHash)},
		contentModerationSideEffectField(subjectKey, effectType),
		contentModerationSideEffectReservationValue(token, time.Time{}),
	).Int64()
	if err != nil {
		return false, err
	}
	return released > 0, nil
}

func (c *contentModerationHashCache) DeleteFlaggedInputHash(ctx context.Context, inputHash string) (bool, error) {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" {
		return false, nil
	}
	deleted, err := deleteContentModerationHashScript.Run(
		ctx,
		c.rdb,
		[]string{contentModerationFlaggedHashKey, contentModerationSideEffectKey(inputHash)},
		inputHash,
	).Int64()
	if err != nil {
		return false, err
	}
	return deleted > 0, nil
}

func (c *contentModerationHashCache) ClearFlaggedInputHashes(ctx context.Context) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, nil
	}
	deleted, err := clearContentModerationHashesScript.Run(
		ctx,
		c.rdb,
		[]string{
			contentModerationFlaggedHashKey,
			contentModerationLegacySideEffectDedupKey,
			contentModerationLegacyFlaggedHashKey,
		},
		contentModerationSideEffectDedupKeyPrefix,
	).Int64()
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

func (c *contentModerationHashCache) CountFlaggedInputHashes(ctx context.Context) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, nil
	}
	return c.rdb.ZCount(ctx, contentModerationFlaggedHashKey, "("+strconv.FormatInt(time.Now().Unix(), 10), "+inf").Result()
}

func contentModerationSideEffectKey(inputHash string) string {
	return contentModerationSideEffectDedupKeyPrefix + strings.TrimSpace(inputHash)
}

func contentModerationSideEffectField(subjectKey string, effectType string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(subjectKey) + "\x00" + strings.TrimSpace(effectType)))
	return hex.EncodeToString(digest[:])
}

func contentModerationSideEffectReservationValue(token string, expiresAt time.Time) string {
	if expiresAt.IsZero() {
		return "r:" + strings.TrimSpace(token) + ":"
	}
	return "r:" + strings.TrimSpace(token) + ":" + strconv.FormatInt(expiresAt.UnixMilli(), 10)
}

func contentModerationSideEffectFinalValue(expiresAt time.Time) string {
	return "f:" + strconv.FormatInt(expiresAt.UnixMilli(), 10)
}
