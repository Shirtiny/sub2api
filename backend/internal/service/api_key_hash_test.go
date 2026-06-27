package service

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyStorageHashUsesHMACSecret(t *testing.T) {
	cfg := &config.Config{}
	cfg.Security.APIKeyHashSecret = "test-hash-secret-with-at-least-32-bytes"
	key := "sk-test-hash-key-123456"

	stored := APIKeyStorageHash(key, cfg)

	require.Equal(t, APIKeyHashAlgHMACSHA256, stored.Alg)
	require.Len(t, stored.Hash, 64)
	require.NotContains(t, stored.Hash, key)

	lookups := APIKeyLookupHashes(key, cfg)
	require.Len(t, lookups, 3)
	require.Equal(t, APIKeyHashAlgLookupSHA256, lookups[0].Alg)
	require.Len(t, lookups[0].Hash, 64)
	require.Equal(t, stored, lookups[1])
	require.Equal(t, APIKeyHashAlgSHA256, lookups[2].Alg)
	require.Len(t, lookups[2].Hash, 64)
}

func TestAPIKeyLookupTokenRoundTrip(t *testing.T) {
	hashes := []APIKeyLookupHash{
		{Alg: APIKeyHashAlgHMACSHA256, Hash: strings.Repeat("a", 64)},
		{Alg: APIKeyHashAlgSHA256, Hash: strings.Repeat("b", 64)},
	}

	token := EncodeAPIKeyLookupToken(hashes)
	decoded, ok := DecodeAPIKeyLookupToken(token)

	require.True(t, ok)
	require.Equal(t, hashes, decoded)
	require.NotContains(t, token, "sk-")
}

func TestAPIKeyPrefixUsesThirtyTwoCharactersForUserScopedCafePassKey(t *testing.T) {
	key := "cafepass-42-1234567890abcdef1234567890abcdef"

	prefix := APIKeyPrefix(key)

	require.Len(t, prefix, 32)
	require.Equal(t, key[:32], prefix)
}

func TestAPIKeyPrefixUsesSixteenCharactersForLegacyCafePassKey(t *testing.T) {
	key := "cafepass-1234567890abcdef"

	prefix := APIKeyPrefix(key)

	require.Len(t, prefix, 16)
	require.Equal(t, key[:16], prefix)
}

func TestAPIKeyPrefixUsesSixteenCharactersForCustomKey(t *testing.T) {
	key := "sk_custom_1234567890"

	prefix := APIKeyPrefix(key)

	require.Equal(t, "sk_custom_123456", prefix)
}

func TestAPIKeyPrefixKeepsVeryShortLegacyKey(t *testing.T) {
	key := "sk-short"

	require.Equal(t, key, APIKeyPrefix(key))
}
