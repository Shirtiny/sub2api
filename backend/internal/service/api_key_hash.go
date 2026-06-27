package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	APIKeyHashAlgHMACSHA256   = "hmac-sha256"
	APIKeyHashAlgSHA256       = "sha256"
	APIKeyHashAlgLookupSHA256 = "lookup-sha256"

	apiKeyDisplayPrefixLen         = 16
	apiKeyUserDisplayPrefixLen     = 32
	apiKeyLookupTokenPrefix        = "__api_key_hash__:"
	apiKeyLegacyAuthCacheKeyPrefix = "legacy-plaintext-sha256:"
)

type APIKeyLookupHash struct {
	Alg  string
	Hash string
}

func APIKeyPrefix(key string) string {
	key = strings.TrimSpace(key)
	prefixLen := apiKeyDisplayPrefixLen
	if isUserScopedCafePassKey(key) {
		prefixLen = apiKeyUserDisplayPrefixLen
	}
	if len(key) <= prefixLen {
		return key
	}
	return key[:prefixLen]
}

func isUserScopedCafePassKey(key string) bool {
	if !strings.HasPrefix(key, "cafepass-") {
		return false
	}
	rest := strings.TrimPrefix(key, "cafepass-")
	uid, _, ok := strings.Cut(rest, "-")
	if !ok || uid == "" {
		return false
	}
	for _, c := range uid {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func APIKeyStorageHash(key string, cfg *config.Config) APIKeyLookupHash {
	key = strings.TrimSpace(key)
	secret := apiKeyHashSecret(cfg)
	if secret == "" {
		return APIKeyLookupHash{Alg: APIKeyHashAlgSHA256, Hash: apiKeySHA256(key)}
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(key))
	return APIKeyLookupHash{Alg: APIKeyHashAlgHMACSHA256, Hash: hex.EncodeToString(mac.Sum(nil))}
}

func APIKeyLookupHashValue(key string) string {
	return apiKeySHA256(strings.TrimSpace(key))
}

func APIKeyLookupHashes(key string, cfg *config.Config) []APIKeyLookupHash {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	out := make([]APIKeyLookupHash, 0, 3)
	out = append(out, APIKeyLookupHash{Alg: APIKeyHashAlgLookupSHA256, Hash: APIKeyLookupHashValue(key)})
	if secret := apiKeyHashSecret(cfg); secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(key))
		out = append(out, APIKeyLookupHash{Alg: APIKeyHashAlgHMACSHA256, Hash: hex.EncodeToString(mac.Sum(nil))})
	}
	out = append(out, APIKeyLookupHash{Alg: APIKeyHashAlgSHA256, Hash: apiKeySHA256(key)})
	return out
}

func EncodeAPIKeyLookupToken(hashes []APIKeyLookupHash) string {
	parts := make([]string, 0, len(hashes))
	for _, h := range hashes {
		alg := strings.TrimSpace(h.Alg)
		hash := strings.TrimSpace(h.Hash)
		if alg == "" || hash == "" {
			continue
		}
		parts = append(parts, alg+"="+hash)
	}
	if len(parts) == 0 {
		return ""
	}
	return apiKeyLookupTokenPrefix + strings.Join(parts, ",")
}

func DecodeAPIKeyLookupToken(token string) ([]APIKeyLookupHash, bool) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, apiKeyLookupTokenPrefix) {
		return nil, false
	}
	body := strings.TrimPrefix(token, apiKeyLookupTokenPrefix)
	if body == "" {
		return nil, true
	}
	rawParts := strings.Split(body, ",")
	hashes := make([]APIKeyLookupHash, 0, len(rawParts))
	for _, part := range rawParts {
		alg, hash, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		alg = strings.TrimSpace(alg)
		hash = strings.TrimSpace(hash)
		if alg == "" || hash == "" {
			continue
		}
		hashes = append(hashes, APIKeyLookupHash{Alg: alg, Hash: hash})
	}
	return hashes, true
}

func apiKeyLegacyAuthCacheKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	return apiKeyLegacyAuthCacheKeyPrefix + APIKeyLookupHashValue(key)
}

func APIKeyAuthCacheKeyFromHash(hash APIKeyLookupHash) string {
	alg := strings.TrimSpace(hash.Alg)
	value := strings.TrimSpace(hash.Hash)
	if alg == "" || value == "" {
		return ""
	}
	return alg + ":" + value
}

func apiKeySHA256(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func apiKeyHashSecret(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if secret := strings.TrimSpace(cfg.Security.APIKeyHashSecret); secret != "" {
		return secret
	}
	return ""
}
