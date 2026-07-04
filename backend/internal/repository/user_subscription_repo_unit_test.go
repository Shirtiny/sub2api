//go:build unit

package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeSubscriptionExpiresAtTreatsZeroAsNoExpirySentinel(t *testing.T) {
	require.Equal(t, subscriptionNoExpirySentinel, normalizeSubscriptionExpiresAt(time.Time{}))

	future := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	require.Equal(t, future, normalizeSubscriptionExpiresAt(future))
}
