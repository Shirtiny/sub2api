package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildAetherWSReconnectDirectiveRunsAdmissionBeforeCreatingClientSignal(t *testing.T) {
	decision := aetherWSRouteControlDecision{
		SignalReconnect:        true,
		ControlID:              "control-a",
		BindingGeneration:      4,
		MiddleRouteDisposition: OpenAIWSMiddleRouteDispositionRetain,
	}
	admissionErr := errors.New("redis unavailable")
	called := false
	hooks := &OpenAIWSIngressHooks{BeforeReconnectSignal: func(control OpenAIWSReconnectControl) error {
		called = true
		require.Equal(t, "control-a", control.ControlID)
		require.Equal(t, uint64(4), control.BindingGeneration)
		require.Equal(t, OpenAIWSMiddleRouteDispositionRetain, control.MiddleRouteDisposition)
		return admissionErr
	}}

	directive := buildAetherWSReconnectDirective(hooks, decision, nil)
	require.True(t, called)
	require.ErrorIs(t, directive.Err, admissionErr)
	require.Empty(t, directive.ClientPayload, "failed admission must not create a Codex reconnect signal")
	require.False(t, directive.Exit)
}

func TestBuildAetherWSReconnectDirectiveEmitsSignalOnlyAfterAdmission(t *testing.T) {
	called := false
	hooks := &OpenAIWSIngressHooks{BeforeReconnectSignal: func(OpenAIWSReconnectControl) error {
		called = true
		return nil
	}}
	directive := buildAetherWSReconnectDirective(hooks, aetherWSRouteControlDecision{
		SignalReconnect:   true,
		ControlID:         "control-a",
		BindingGeneration: 4,
	}, nil)

	require.True(t, called)
	require.JSONEq(t, string(aetherWSReconnectErrorPayload()), string(directive.ClientPayload))
	require.True(t, directive.Exit)
	require.ErrorIs(t, directive.Err, ErrOpenAIWSReconnectMigrationRequested)
}

func TestBuildAetherWSReconnectDirectiveInitialFailoverDoesNotRecordMigration(t *testing.T) {
	called := false
	hooks := &OpenAIWSIngressHooks{BeforeReconnectSignal: func(OpenAIWSReconnectControl) error {
		called = true
		return nil
	}}
	headers := make(http.Header)
	headers.Set("x-request-id", "request-a")
	directive := buildAetherWSReconnectDirective(hooks, aetherWSRouteControlDecision{
		SignalReconnect:        true,
		InitialStepFailover:    true,
		MiddleRouteDisposition: OpenAIWSMiddleRouteDispositionRetain,
	}, headers)

	require.False(t, called)
	require.Empty(t, directive.ClientPayload)
	var failover *OpenAIWSInitialStepFailoverError
	require.ErrorAs(t, directive.Err, &failover)
	require.True(t, failover.FailoverError().DoNotPenalizeAccount)
	require.Equal(t, OpenAIWSMiddleRouteDispositionRetain, failover.FailoverError().MiddleRouteDisposition)
}

func TestBuildAetherWSReconnectDirectiveInitialExcludePenalizesAndCarriesRetryHint(t *testing.T) {
	directive := buildAetherWSReconnectDirective(nil, aetherWSRouteControlDecision{
		SignalReconnect:        true,
		InitialStepFailover:    true,
		RetryAfterMS:           250,
		MiddleRouteDisposition: OpenAIWSMiddleRouteDispositionExclude,
	}, nil)

	var failover *OpenAIWSInitialStepFailoverError
	require.ErrorAs(t, directive.Err, &failover)
	require.False(t, failover.FailoverError().DoNotPenalizeAccount)
	require.Equal(t, 250, failover.FailoverError().RetryAfterMS)
	require.Equal(t, OpenAIWSMiddleRouteDispositionExclude, failover.FailoverError().MiddleRouteDisposition)
}
