package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRunOpenAIWSUsageRecordWithRetry(t *testing.T) {
	attempts := 0
	err := runOpenAIWSUsageRecordWithRetry(context.Background(), func(context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, attempts)
}

func TestRunOpenAIWSUsageRecordWithRetryStopsOnFingerprintConflict(t *testing.T) {
	attempts := 0
	err := runOpenAIWSUsageRecordWithRetry(context.Background(), func(context.Context) error {
		attempts++
		return service.ErrUsageBillingRequestConflict
	})
	require.ErrorIs(t, err, service.ErrUsageBillingRequestConflict)
	require.Equal(t, 1, attempts)
}
