package repository

import (
	"context"
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
	contentModerationFlaggedHashKey       = "content_moderation:flagged_hashes:v2"
	contentModerationLegacyFlaggedHashKey = "content_moderation:flagged_hashes"
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

func (c *contentModerationHashCache) DeleteFlaggedInputHash(ctx context.Context, inputHash string) (bool, error) {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" {
		return false, nil
	}
	deleted, err := c.rdb.ZRem(ctx, contentModerationFlaggedHashKey, inputHash).Result()
	if err != nil {
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
	if err := c.rdb.Del(ctx, contentModerationFlaggedHashKey, contentModerationLegacyFlaggedHashKey).Err(); err != nil {
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
