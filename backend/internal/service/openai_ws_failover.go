package service

import (
	"errors"
	"net/http"
)

// OpenAIWSInitialStepFailoverError is the only WebSocket proxy error that may
// restart account selection. A plain UpstreamFailoverError is intentionally not
// sufficient: once a later step has started, the outer handler no longer owns
// that step's payload and can only replay the connection's initial frame.
type OpenAIWSInitialStepFailoverError struct {
	failoverErr *UpstreamFailoverError
}

func (e *OpenAIWSInitialStepFailoverError) Error() string {
	if e == nil || e.failoverErr == nil {
		return "openai websocket initial step failover"
	}
	return e.failoverErr.Error()
}

func (e *OpenAIWSInitialStepFailoverError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.failoverErr
}

// FailoverError returns the scheduling error carried by this WS-specific
// initial-step fence.
func (e *OpenAIWSInitialStepFailoverError) FailoverError() *UpstreamFailoverError {
	if e == nil {
		return nil
	}
	return e.failoverErr
}

// NewOpenAIWSInitialStepFailoverError marks a failure as safe for the WS
// handler to retry on another account. Callers must only use it before a later
// client step can have been consumed.
func NewOpenAIWSInitialStepFailoverError(failoverErr *UpstreamFailoverError) error {
	if failoverErr == nil {
		return nil
	}
	return &OpenAIWSInitialStepFailoverError{failoverErr: failoverErr}
}

func fenceOpenAIWSFailoverByStep(step int, err error) error {
	if err == nil || step != 1 {
		return err
	}
	var alreadyFenced *OpenAIWSInitialStepFailoverError
	if errors.As(err, &alreadyFenced) {
		return err
	}
	var failoverErr *UpstreamFailoverError
	if !errors.As(err, &failoverErr) || failoverErr == nil {
		return err
	}
	return NewOpenAIWSInitialStepFailoverError(failoverErr)
}

// openAIWSRateLimitFailoverError keeps trusted middle-hop dispatch proof on the
// route-v1 control channel. A provider-style 429 received through Aether does
// not prove that the official provider never executed the request, so it must
// be forwarded to the client instead of triggering an outer first-frame replay.
func openAIWSRateLimitFailoverError(step int, requiresRouteControlProof bool, failoverErr *UpstreamFailoverError) error {
	if failoverErr == nil || requiresRouteControlProof {
		return nil
	}
	return fenceOpenAIWSFailoverByStep(step, failoverErr)
}

// newOpenAIWSPreDispatchFailover converts a failure that happened before the
// first provider-bound business frame into the only error the outer account
// loop may retry. The caller logs the underlying cause before conversion.
func newOpenAIWSPreDispatchFailover(statusCode int, headers http.Header) error {
	if statusCode <= 0 || statusCode == http.StatusSwitchingProtocols {
		statusCode = http.StatusBadGateway
	}
	return NewOpenAIWSInitialStepFailoverError(&UpstreamFailoverError{
		StatusCode:      statusCode,
		ResponseHeaders: cloneHeader(headers),
	})
}
