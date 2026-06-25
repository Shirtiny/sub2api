package service

import (
	"context"
	"strings"
)

// InvalidateAuthCacheByKey 清除指定 API Key 的认证缓存
func (s *APIKeyService) InvalidateAuthCacheByKey(ctx context.Context, key string) {
	if key == "" {
		return
	}
	if hashes, ok := DecodeAPIKeyLookupToken(key); ok {
		s.invalidateAuthCacheByHashes(ctx, hashes)
		return
	}
	s.invalidateAuthCacheByHashes(ctx, APIKeyLookupHashes(key, s.cfg))
}

// InvalidateAuthCacheByUserID 清除用户相关的 API Key 认证缓存
func (s *APIKeyService) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	if userID <= 0 {
		return
	}
	keys, err := s.apiKeyRepo.ListKeysByUserID(ctx, userID)
	if err != nil {
		return
	}
	s.deleteAuthCacheByKeys(ctx, keys)
}

// InvalidateAuthCacheByGroupID 清除分组相关的 API Key 认证缓存
func (s *APIKeyService) InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64) {
	if groupID <= 0 {
		return
	}
	keys, err := s.apiKeyRepo.ListKeysByGroupID(ctx, groupID)
	if err != nil {
		return
	}
	s.deleteAuthCacheByKeys(ctx, keys)
}

func (s *APIKeyService) InvalidateAuthCacheByHash(ctx context.Context, alg, hash string) {
	s.invalidateAuthCacheByHashes(ctx, []APIKeyLookupHash{{Alg: alg, Hash: hash}})
}

func (s *APIKeyService) invalidateAuthCacheByAPIKey(ctx context.Context, apiKey *APIKey) {
	if apiKey == nil {
		return
	}
	hashes := apiKeyAuthCacheHashes(apiKey.KeyLookupHash, apiKey.KeyHashAlg, apiKey.KeyHash)
	if len(hashes) > 0 {
		s.invalidateAuthCacheByHashes(ctx, hashes)
		return
	}
	s.InvalidateAuthCacheByKey(ctx, apiKey.Key)
}

func (s *APIKeyService) invalidateAuthCacheByQuotaState(ctx context.Context, state *APIKeyQuotaUsageState) {
	if state == nil || state.Status != StatusAPIKeyQuotaExhausted {
		return
	}
	hashes := apiKeyAuthCacheHashes(state.KeyLookupHash, state.KeyHashAlg, state.KeyHash)
	if len(hashes) > 0 {
		s.invalidateAuthCacheByHashes(ctx, hashes)
		return
	}
	s.InvalidateAuthCacheByKey(ctx, state.Key)
}

func apiKeyAuthCacheHashes(keyLookupHash, keyHashAlg, keyHash string) []APIKeyLookupHash {
	hashes := make([]APIKeyLookupHash, 0, 2)
	keyLookupHash = strings.TrimSpace(keyLookupHash)
	if keyLookupHash != "" {
		hashes = append(hashes, APIKeyLookupHash{
			Alg:  APIKeyHashAlgLookupSHA256,
			Hash: keyLookupHash,
		})
	}
	keyHash = strings.TrimSpace(keyHash)
	keyHashAlg = strings.TrimSpace(keyHashAlg)
	if keyHashAlg == "" {
		keyHashAlg = APIKeyHashAlgSHA256
	}
	if keyHash != "" {
		hashes = append(hashes, APIKeyLookupHash{
			Alg:  keyHashAlg,
			Hash: keyHash,
		})
	}
	return hashes
}

func (s *APIKeyService) deleteAuthCacheByKeys(ctx context.Context, keys []string) {
	if len(keys) == 0 {
		return
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		s.InvalidateAuthCacheByKey(ctx, key)
	}
}

func (s *APIKeyService) invalidateAuthCacheByHashes(ctx context.Context, hashes []APIKeyLookupHash) {
	for _, hash := range hashes {
		cacheKey := APIKeyAuthCacheKeyFromHash(hash)
		if cacheKey == "" {
			continue
		}
		s.deleteAuthCache(ctx, cacheKey)
	}
}
