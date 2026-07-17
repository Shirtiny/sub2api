package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	apiKeyRateLimitKeyPrefix   = "apikey:ratelimit:"
	apiKeyRateLimitDuration    = 24 * time.Hour
	apiKeyAuthCachePrefix      = "apikey:auth:"
	apiKeyAuthEpochPrefix      = "apikey:auth:epoch:{generation}:"
	apiKeyAuthEpochSequenceKey = apiKeyAuthEpochPrefix + "sequence"
	apiKeyAuthEpochTTL         = 30 * 24 * time.Hour
	authCacheInvalidateChannel = "auth:cache:invalidate"
)

var incrementAuthCacheEpochScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[2]) == 0 then
  local now = redis.call('TIME')
  local seed = now[1] .. string.format('%06d', now[2])
  redis.call('SET', KEYS[2], seed, 'NX')
end
local epoch = redis.call('INCR', KEYS[2])
redis.call('SET', KEYS[1], epoch, 'PX', ARGV[1])
return epoch
`)

var initializeAuthCacheEpochScript = redis.NewScript(`
local epoch = redis.call('GET', KEYS[1])
if epoch then
  return epoch
end
if redis.call('EXISTS', KEYS[2]) == 0 then
  local now = redis.call('TIME')
  local seed = now[1] .. string.format('%06d', now[2])
  redis.call('SET', KEYS[2], seed, 'NX')
end
epoch = redis.call('INCR', KEYS[2])
redis.call('SET', KEYS[1], epoch, 'PX', ARGV[1])
return epoch
`)

// apiKeyRateLimitKey generates the Redis key for API key creation rate limiting.
func apiKeyRateLimitKey(userID int64) string {
	return fmt.Sprintf("%s%d", apiKeyRateLimitKeyPrefix, userID)
}

func apiKeyAuthCacheKey(key string) string {
	return fmt.Sprintf("%s%s", apiKeyAuthCachePrefix, key)
}

type apiKeyCache struct {
	rdb *redis.Client
}

func NewAPIKeyCache(rdb *redis.Client) service.APIKeyCache {
	return &apiKeyCache{rdb: rdb}
}

func (c *apiKeyCache) GetCreateAttemptCount(ctx context.Context, userID int64) (int, error) {
	key := apiKeyRateLimitKey(userID)
	count, err := c.rdb.Get(ctx, key).Int()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return count, err
}

func (c *apiKeyCache) IncrementCreateAttemptCount(ctx context.Context, userID int64) error {
	key := apiKeyRateLimitKey(userID)
	pipe := c.rdb.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, apiKeyRateLimitDuration)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *apiKeyCache) DeleteCreateAttemptCount(ctx context.Context, userID int64) error {
	key := apiKeyRateLimitKey(userID)
	return c.rdb.Del(ctx, key).Err()
}

func (c *apiKeyCache) IncrementDailyUsage(ctx context.Context, apiKey string) error {
	return c.rdb.Incr(ctx, apiKey).Err()
}

func (c *apiKeyCache) SetDailyUsageExpiry(ctx context.Context, apiKey string, ttl time.Duration) error {
	return c.rdb.Expire(ctx, apiKey, ttl).Err()
}

func (c *apiKeyCache) GetAuthCache(ctx context.Context, key string) (*service.APIKeyAuthCacheEntry, error) {
	val, err := c.rdb.Get(ctx, apiKeyAuthCacheKey(key)).Bytes()
	if err != nil {
		return nil, err
	}
	var entry service.APIKeyAuthCacheEntry
	if err := json.Unmarshal(val, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (c *apiKeyCache) SetAuthCache(ctx context.Context, key string, entry *service.APIKeyAuthCacheEntry, ttl time.Duration) error {
	if entry == nil {
		return nil
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, apiKeyAuthCacheKey(key), payload, ttl).Err()
}

func (c *apiKeyCache) DeleteAuthCache(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, apiKeyAuthCacheKey(key)).Err()
}

func (c *apiKeyCache) GetAuthCacheEpoch(ctx context.Context, cacheKey string) (uint64, error) {
	key := apiKeyAuthEpochPrefix + cacheKey
	value, err := c.rdb.Get(ctx, key).Uint64()
	if err == nil {
		return value, nil
	}
	if !errors.Is(err, redis.Nil) {
		return 0, err
	}

	// A missing per-key generation must not map back to zero: doing so would
	// allow an old zero-valued lease to validate after an invalidation key's TTL
	// elapsed. Allocate from one permanent global sequence instead. The hash tag
	// keeps both keys in the same Redis Cluster slot.
	return initializeAuthCacheEpochScript.Run(
		ctx,
		c.rdb,
		[]string{key, apiKeyAuthEpochSequenceKey},
		apiKeyAuthEpochTTL.Milliseconds(),
	).Uint64()
}

func (c *apiKeyCache) IncrementAuthCacheEpoch(ctx context.Context, cacheKey string) error {
	return incrementAuthCacheEpochScript.Run(
		ctx,
		c.rdb,
		[]string{apiKeyAuthEpochPrefix + cacheKey, apiKeyAuthEpochSequenceKey},
		apiKeyAuthEpochTTL.Milliseconds(),
	).Err()
}

// PublishAuthCacheInvalidation publishes a cache invalidation message to all instances
func (c *apiKeyCache) PublishAuthCacheInvalidation(ctx context.Context, cacheKey string) error {
	return c.rdb.Publish(ctx, authCacheInvalidateChannel, cacheKey).Err()
}

// SubscribeAuthCacheInvalidation subscribes to cache invalidation messages
func (c *apiKeyCache) SubscribeAuthCacheInvalidation(ctx context.Context, handler func(cacheKey string)) error {
	pubsub := c.rdb.Subscribe(ctx, authCacheInvalidateChannel)

	// Verify subscription is working
	_, err := pubsub.Receive(ctx)
	if err != nil {
		_ = pubsub.Close()
		return fmt.Errorf("subscribe to auth cache invalidation: %w", err)
	}

	go func() {
		defer func() {
			if err := pubsub.Close(); err != nil {
				log.Printf("Warning: failed to close auth cache invalidation pubsub: %v", err)
			}
		}()

		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if msg != nil {
					handler(msg.Payload)
				}
			}
		}
	}()

	return nil
}
