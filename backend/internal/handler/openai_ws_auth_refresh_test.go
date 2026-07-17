package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func validOpenAIWSRefreshedAPIKey() *service.APIKey {
	return &service.APIKey{
		Status: service.StatusAPIKeyActive,
		User:   &service.User{Status: service.StatusActive},
	}
}

func TestValidateOpenAIWSRefreshedAPIKey(t *testing.T) {
	t.Run("active", func(t *testing.T) {
		status, code, _, ok := validateOpenAIWSRefreshedAPIKey(validOpenAIWSRefreshedAPIKey(), "127.0.0.1")
		require.True(t, ok)
		require.Zero(t, status)
		require.Empty(t, code)
	})

	t.Run("disabled key", func(t *testing.T) {
		key := validOpenAIWSRefreshedAPIKey()
		key.Status = service.StatusAPIKeyDisabled
		status, code, _, ok := validateOpenAIWSRefreshedAPIKey(key, "127.0.0.1")
		require.False(t, ok)
		require.Equal(t, http.StatusUnauthorized, status)
		require.Equal(t, "API_KEY_DISABLED", code)
	})

	t.Run("runtime expiry", func(t *testing.T) {
		key := validOpenAIWSRefreshedAPIKey()
		expiredAt := time.Now().Add(-time.Minute)
		key.ExpiresAt = &expiredAt
		status, code, _, ok := validateOpenAIWSRefreshedAPIKey(key, "127.0.0.1")
		require.False(t, ok)
		require.Equal(t, http.StatusForbidden, status)
		require.Equal(t, "API_KEY_EXPIRED", code)
	})

	t.Run("inactive user", func(t *testing.T) {
		key := validOpenAIWSRefreshedAPIKey()
		key.User.Status = "disabled"
		status, code, _, ok := validateOpenAIWSRefreshedAPIKey(key, "127.0.0.1")
		require.False(t, ok)
		require.Equal(t, http.StatusUnauthorized, status)
		require.Equal(t, "USER_INACTIVE", code)
	})

	t.Run("disabled group", func(t *testing.T) {
		key := validOpenAIWSRefreshedAPIKey()
		groupID := int64(7)
		key.GroupID = &groupID
		key.Group = &service.Group{ID: groupID, Status: "disabled"}
		status, code, _, ok := validateOpenAIWSRefreshedAPIKey(key, "127.0.0.1")
		require.False(t, ok)
		require.Equal(t, http.StatusForbidden, status)
		require.Equal(t, "GROUP_DISABLED", code)
	})

	t.Run("ip acl", func(t *testing.T) {
		key := validOpenAIWSRefreshedAPIKey()
		key.IPWhitelist = []string{"10.0.0.0/8"}
		key.CompiledIPWhitelist = ip.CompileIPRules(key.IPWhitelist)
		status, code, _, ok := validateOpenAIWSRefreshedAPIKey(key, "192.0.2.1")
		require.False(t, ok)
		require.Equal(t, http.StatusForbidden, status)
		require.Equal(t, "ACCESS_DENIED", code)
	})
}

func TestSameOptionalInt64(t *testing.T) {
	left := int64(1)
	right := int64(1)
	other := int64(2)
	require.True(t, sameOptionalInt64(nil, nil))
	require.True(t, sameOptionalInt64(&left, &right))
	require.False(t, sameOptionalInt64(&left, nil))
	require.False(t, sameOptionalInt64(&left, &other))
}
