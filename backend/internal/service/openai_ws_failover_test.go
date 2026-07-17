package service

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFenceOpenAIWSFailoverByStep(t *testing.T) {
	t.Parallel()

	t.Run("initial step is explicitly switchable", func(t *testing.T) {
		failoverErr := &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}
		got := fenceOpenAIWSFailoverByStep(1, fmt.Errorf("route failed: %w", failoverErr))

		var initialStepErr *OpenAIWSInitialStepFailoverError
		require.ErrorAs(t, got, &initialStepErr)
		require.Same(t, failoverErr, initialStepErr.FailoverError())
	})

	t.Run("second step 429 is not switchable", func(t *testing.T) {
		failoverErr := &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}
		got := fenceOpenAIWSFailoverByStep(2, failoverErr)

		var initialStepErr *OpenAIWSInitialStepFailoverError
		require.False(t, errors.As(got, &initialStepErr))
		require.Same(t, failoverErr, got)
	})

	t.Run("second step wrapped route failure is not switchable", func(t *testing.T) {
		failoverErr := &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable}
		routeErr := fmt.Errorf("aether route failure: %w", failoverErr)
		got := fenceOpenAIWSFailoverByStep(2, routeErr)

		var initialStepErr *OpenAIWSInitialStepFailoverError
		require.False(t, errors.As(got, &initialStepErr))
		require.Same(t, routeErr, got)
	})
}

func TestNewOpenAIWSPreDispatchFailoverNormalizesMissingAnd101Status(t *testing.T) {
	for _, statusCode := range []int{0, http.StatusSwitchingProtocols} {
		err := newOpenAIWSPreDispatchFailover(statusCode, http.Header{"X-Request-Id": {"req-1"}})
		var fenced *OpenAIWSInitialStepFailoverError
		require.ErrorAs(t, err, &fenced)
		require.Equal(t, http.StatusBadGateway, fenced.FailoverError().StatusCode)
		require.Equal(t, "req-1", fenced.FailoverError().ResponseHeaders.Get("X-Request-Id"))
	}
}

func TestOpenAIWSRateLimitFailoverRequiresRouteControlProofForAether(t *testing.T) {
	failoverErr := &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}

	direct := openAIWSRateLimitFailoverError(1, false, failoverErr)
	var initial *OpenAIWSInitialStepFailoverError
	require.ErrorAs(t, direct, &initial)

	require.NoError(t, openAIWSRateLimitFailoverError(1, true, failoverErr),
		"an unproven Aether 429 must be forwarded instead of replaying the first frame")
	require.Same(t, failoverErr, openAIWSRateLimitFailoverError(2, false, failoverErr))
}
