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
	contentModerationFlaggedHashKey          = "content_moderation:flagged_hashes:v2"
	contentModerationSideEffectDedupKey      = "content_moderation:side_effect_dedup:v1"
	contentModerationLegacyFlaggedHashKey    = "content_moderation:flagged_hashes"
	contentModerationSideEffectScanBatchSize = 256
)

var claimContentModerationSideEffectScript = redis.NewScript(`
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
return redis.call('ZADD', KEYS[1], 'NX', ARGV[2], ARGV[3])
`)

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

// ClaimFlaggedInputSideEffects atomically claims the notification and violation
// side effects for one authenticated subject and one globally flagged input.
// The global input hash remains shared so known-bad content can still skip the
// moderation API, while the subject-scoped claim prevents one user from
// suppressing another user's notification and violation count.
func (c *contentModerationHashCache) ClaimFlaggedInputSideEffects(ctx context.Context, subjectKey string, inputHash string, ttl time.Duration) (bool, error) {
	subjectKey = strings.TrimSpace(subjectKey)
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || subjectKey == "" || inputHash == "" || ttl <= 0 {
		return false, nil
	}

	now := time.Now()
	claimed, err := claimContentModerationSideEffectScript.Run(
		ctx,
		c.rdb,
		[]string{contentModerationSideEffectDedupKey},
		now.Unix(),
		now.Add(ttl).Unix(),
		contentModerationSideEffectMember(subjectKey, inputHash),
	).Int64()
	if err != nil {
		return false, err
	}
	return claimed > 0, nil
}

func (c *contentModerationHashCache) DeleteFlaggedInputHash(ctx context.Context, inputHash string) (bool, error) {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" {
		return false, nil
	}
	deleted, err := c.rdb.ZRem(ctx, contentModerationFlaggedHashKey, inputHash).Result()
	if err != nil {
		return false, err
	}
	if err := c.deleteSideEffectClaimsForInputHash(ctx, inputHash); err != nil {
		return false, err
	}
	return deleted > 0, nil
}

func (c *contentModerationHashCache) ClearFlaggedInputHashes(ctx context.Context) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, nil
	}
	deleted, err := c.rdb.ZCard(ctx, contentModerationFlaggedHashKey).Result()
	if err != nil {
		return 0, err
	}
	if err := c.rdb.Del(
		ctx,
		contentModerationFlaggedHashKey,
		contentModerationSideEffectDedupKey,
		contentModerationLegacyFlaggedHashKey,
	).Err(); err != nil {
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

func contentModerationSideEffectMember(subjectKey string, inputHash string) string {
	subjectDigest := sha256.Sum256([]byte(strings.TrimSpace(subjectKey)))
	return strings.TrimSpace(inputHash) + ":" + hex.EncodeToString(subjectDigest[:])
}

func (c *contentModerationHashCache) deleteSideEffectClaimsForInputHash(ctx context.Context, inputHash string) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	pattern := strings.TrimSpace(inputHash) + ":*"
	var cursor uint64
	for {
		items, next, err := c.rdb.ZScan(
			ctx,
			contentModerationSideEffectDedupKey,
			cursor,
			pattern,
			contentModerationSideEffectScanBatchSize,
		).Result()
		if err != nil {
			return err
		}
		members := make([]any, 0, (len(items)+1)/2)
		for index := 0; index < len(items); index += 2 {
			members = append(members, items[index])
		}
		if len(members) > 0 {
			if err := c.rdb.ZRem(ctx, contentModerationSideEffectDedupKey, members...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}
