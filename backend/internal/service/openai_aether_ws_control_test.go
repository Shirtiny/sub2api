package service

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newAetherWSRouteControlTestConsumer(t *testing.T, _ bool) *aetherWSRouteControlConsumer {
	t.Helper()
	consumer, err := newAetherWSRouteControlConsumer(aetherWSRouteControlConsumerConfig{
		Negotiated: AetherWSNegotiatedCapabilities{
			ControlProtocol:    AetherWSControlProtocolRouteV1,
			CloseAfterTerminal: true,
			ClientReconnect:    true,
		},
		ReconnectEnabled:    true,
		ReconnectSignalMode: "websocket_connection_limit_reached",
		BindingEpochID:      "binding-epoch-test",
		BindingGeneration:   7,
	})
	require.NoError(t, err)
	return consumer
}

func prepareAetherWSRouteControlTestStep(t *testing.T, consumer *aetherWSRouteControlConsumer) ([]byte, aetherWSStepFence) {
	t.Helper()
	prepared, err := consumer.prepareResponseCreate([]byte(`{"type":"response.create","model":"gpt-5","client_metadata":{"keep":"yes","aether.sub2api_step_control":{"sub2api_step_correlation_id":"spoof"}}}`))
	require.NoError(t, err)
	var fence aetherWSStepFence
	fenceRaw := gjson.GetBytes(prepared, "client_metadata.aether\\.sub2api_step_control").Raw
	require.NotEmpty(t, fenceRaw)
	require.NoError(t, json.Unmarshal([]byte(fenceRaw), &fence))
	return prepared, fence
}

func TestAetherWSRouteControlAllowsExactPublicLimitPlusFence(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	prefix := []byte(`{"type":"response.create","model":"gpt-5","input":"`)
	suffix := []byte(`"}`)
	paddingLen := AetherWSMaxClientPayloadBytes - len(prefix) - len(suffix)
	require.Positive(t, paddingLen)
	payload := make([]byte, 0, AetherWSMaxClientPayloadBytes)
	payload = append(payload, prefix...)
	payload = append(payload, bytes.Repeat([]byte{'x'}, paddingLen)...)
	payload = append(payload, suffix...)
	require.Len(t, payload, AetherWSMaxClientPayloadBytes)
	require.NoError(t, validateAetherWSClientPayload(payload))

	prepared, err := consumer.prepareValidatedResponseCreate(payload)
	require.NoError(t, err)
	require.Greater(t, len(prepared), AetherWSMaxClientPayloadBytes)
	require.NoError(t, validateAetherWSRoutedPayload(prepared))
}

func aetherWSRouteControlTestFrame(fence aetherWSStepFence, action string) aetherWSRouteControlFrame {
	frame := aetherWSRouteControlFrame{
		Type:                         aetherWSRouteControlEventType,
		Version:                      aetherWSRouteControlVersion,
		Action:                       action,
		ControlID:                    "control-1",
		Reason:                       "account_unavailable",
		Sub2APIStepCorrelationID:     fence.Sub2APIStepCorrelationID,
		Sub2APIBindingEpochID:        fence.Sub2APIBindingEpochID,
		Sub2APIBindingGeneration:     fence.Sub2APIBindingGeneration,
		AetherStepID:                 "aether-step-1",
		AetherAttemptID:              "aether-attempt-1",
		RetryAfterMS:                 250,
		RecommendedAction:            action,
		ProviderExecutionDisposition: "not_dispatched",
	}
	if action == aetherWSRouteActionCloseAfterTerminal {
		frame.Scope = "next_binding"
		frame.EffectiveAfter = "current_terminal"
		frame.CurrentAttemptState = "terminal"
		frame.ProviderWriteState = "confirmed"
		frame.ProviderExecutionDisposition = "terminal"
	} else {
		middleRouteDisposition := OpenAIWSMiddleRouteDispositionRetain
		frame.MiddleRouteDisposition = &middleRouteDisposition
		frame.Scope = "current_step"
		frame.EffectiveAfter = "immediate"
		frame.CurrentAttemptState = "prepared"
		frame.ProviderWriteState = "not_started"
	}
	return frame
}

func TestAetherWSRouteControl_ClientReconnectRequiresValidMiddleRouteDisposition(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	_, fence := prepareAetherWSRouteControlTestStep(t, consumer)

	for name, disposition := range map[string]*OpenAIWSMiddleRouteDisposition{
		"missing": nil,
		"invalid": func() *OpenAIWSMiddleRouteDisposition {
			value := OpenAIWSMiddleRouteDisposition("replace")
			return &value
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			frame := aetherWSRouteControlTestFrame(fence, aetherWSRouteActionClientReconnect)
			frame.MiddleRouteDisposition = disposition
			consumed, _, err := consumer.consumeUpstreamFrame(marshalAetherWSRouteControlTestFrame(t, frame))
			require.True(t, consumed)
			require.ErrorContains(t, err, "middle route disposition")
		})
	}
}

func TestAetherWSRouteControl_CloseAfterTerminalRejectsMiddleRouteDisposition(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	_, fence := prepareAetherWSRouteControlTestStep(t, consumer)
	frame := aetherWSRouteControlTestFrame(fence, aetherWSRouteActionCloseAfterTerminal)
	disposition := OpenAIWSMiddleRouteDispositionRetain
	frame.MiddleRouteDisposition = &disposition

	consumed, _, err := consumer.consumeUpstreamFrame(marshalAetherWSRouteControlTestFrame(t, frame))
	require.True(t, consumed)
	require.ErrorContains(t, err, "proof is invalid")
}

func marshalAetherWSRouteControlTestFrame(t *testing.T, frame aetherWSRouteControlFrame) []byte {
	t.Helper()
	payload, err := json.Marshal(frame)
	require.NoError(t, err)
	return payload
}

func TestAetherWSRouteControl_PrepareResponseCreateOverwritesReservedFence(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	prepared, firstFence := prepareAetherWSRouteControlTestStep(t, consumer)

	require.Equal(t, "yes", gjson.GetBytes(prepared, "client_metadata.keep").String())
	require.Equal(t, aetherWSRouteControlVersion, firstFence.Version)
	require.NotEqual(t, "spoof", firstFence.Sub2APIStepCorrelationID)
	require.True(t, validAetherWSOpaqueID(firstFence.Sub2APIStepCorrelationID))
	require.Equal(t, "binding-epoch-test", firstFence.Sub2APIBindingEpochID)
	require.Equal(t, uint64(7), firstFence.Sub2APIBindingGeneration)

	second, secondFence := prepareAetherWSRouteControlTestStep(t, consumer)
	require.NotEqual(t, firstFence.Sub2APIStepCorrelationID, secondFence.Sub2APIStepCorrelationID)
	require.Equal(t, firstFence.Sub2APIBindingEpochID, secondFence.Sub2APIBindingEpochID)
	require.Equal(t, "yes", gjson.GetBytes(second, "client_metadata.keep").String())

	primitiveMetadata, err := consumer.prepareResponseCreate([]byte(`{"type":"response.create","client_metadata":"spoof"}`))
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(primitiveMetadata, "client_metadata").IsObject())
	require.NotEmpty(t, gjson.GetBytes(primitiveMetadata, "client_metadata.aether\\.sub2api_step_control.sub2api_step_correlation_id").String())
}

func TestAetherWSRouteControl_OrdinaryFrameUsesFastNegativePath(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	payload := []byte(`{"type":"response.output_text.delta","delta":"text mentioning aether.route_control is ordinary output"}`)
	consumed, decision, err := consumer.consumeUpstreamFrame(payload)
	require.NoError(t, err)
	require.False(t, consumed)
	require.Equal(t, aetherWSRouteControlDecision{}, decision)
}

func TestAetherWSRouteControl_EscapedAndDuplicateReservedTypesCannotBypassConsumer(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	_, fence := prepareAetherWSRouteControlTestStep(t, consumer)
	valid := marshalAetherWSRouteControlTestFrame(t, aetherWSRouteControlTestFrame(fence, aetherWSRouteActionClientReconnect))

	escapedDot := bytes.Replace(valid, []byte(`aether.route_control`), []byte(`aether\u002eroute_control`), 1)
	consumed, decision, err := consumer.consumeUpstreamFrame(escapedDot)
	require.NoError(t, err)
	require.True(t, consumed)
	require.True(t, decision.SignalReconnect)

	escapedKey := bytes.Replace(valid, []byte(`"type"`), []byte(`"ty\u0070e"`), 1)
	consumed, _, err = consumer.consumeUpstreamFrame(escapedKey)
	require.NoError(t, err)
	require.True(t, consumed)

	allUnicode := bytes.Replace(valid, []byte(`aether.route_control`), []byte(`\u0061\u0065\u0074\u0068\u0065\u0072\u002e\u0072\u006f\u0075\u0074\u0065\u005f\u0063\u006f\u006e\u0074\u0072\u006f\u006c`), 1)
	consumed, _, err = consumer.consumeUpstreamFrame(allUnicode)
	require.NoError(t, err)
	require.True(t, consumed)

	ordinaryThenReserved := append([]byte(`{"type":"response.output_text.delta",`), valid[1:]...)
	reservedThenOrdinary := append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"type":"response.output_text.delta"}`)...)
	for name, payload := range map[string][]byte{
		"ordinary then reserved": ordinaryThenReserved,
		"reserved then ordinary": reservedThenOrdinary,
	} {
		t.Run(name, func(t *testing.T) {
			consumed, _, err := consumer.consumeUpstreamFrame(payload)
			require.True(t, consumed)
			require.ErrorContains(t, err, "type field is duplicated")
		})
	}
}

func TestAetherWSRouteControl_NestedReservedMarkerRemainsOrdinaryProviderData(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	for _, payload := range [][]byte{
		[]byte(`{"type":"response.output_text.delta","delta":"aether.route_control","nested":{"type":"aether.route_control"}}`),
		[]byte(`{"type":"response.output_text.delta","nested":{"ty\u0070e":"aether\u002eroute_control"}}`),
	} {
		consumed, decision, err := consumer.consumeUpstreamFrame(payload)
		require.NoError(t, err)
		require.False(t, consumed)
		require.Equal(t, aetherWSRouteControlDecision{}, decision)
	}
}

func TestAetherWSRouteControl_ClientReconnectPreparedInitialStep(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	_, fence := prepareAetherWSRouteControlTestStep(t, consumer)
	frame := aetherWSRouteControlTestFrame(fence, aetherWSRouteActionClientReconnect)

	consumed, decision, err := consumer.consumeUpstreamFrame(marshalAetherWSRouteControlTestFrame(t, frame))
	require.NoError(t, err)
	require.True(t, consumed)
	require.True(t, decision.SignalReconnect)
	require.True(t, decision.InitialStepFailover)
	require.False(t, decision.CloseAfterTerminal)
	require.Equal(t, frame.ControlID, decision.ControlID)
	require.Equal(t, fence.Sub2APIBindingGeneration, decision.BindingGeneration)
	require.Equal(t, frame.RetryAfterMS, decision.RetryAfterMS)
}

func TestAetherWSRouteControl_ClientReconnectProvenNotExecuted(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	_, fence := prepareAetherWSRouteControlTestStep(t, consumer)
	frame := aetherWSRouteControlTestFrame(fence, aetherWSRouteActionClientReconnect)
	frame.CurrentAttemptState = "rejected_before_execution"
	frame.ProviderWriteState = "confirmed"
	frame.ProviderExecutionDisposition = "proven_not_executed"
	proofClass := AetherWSAdapterProofClassCodexOfficialNotExecuted
	proofVersion := AetherWSAdapterProofVersionV1
	frame.AdapterProofClass = &proofClass
	frame.AdapterProofVersion = &proofVersion

	consumed, decision, err := consumer.consumeUpstreamFrame(marshalAetherWSRouteControlTestFrame(t, frame))
	require.NoError(t, err)
	require.True(t, consumed)
	require.True(t, decision.SignalReconnect)
}

func TestAetherWSRouteControl_ClientReconnectRejectsUnsafeProofOrCommittedOutput(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*aetherWSRouteControlConsumer, *aetherWSRouteControlFrame)
	}{
		{name: "provider output committed", mutate: func(c *aetherWSRouteControlConsumer, _ *aetherWSRouteControlFrame) { c.markProviderFrameWritten(false) }},
		{name: "accepted disposition", mutate: func(_ *aetherWSRouteControlConsumer, f *aetherWSRouteControlFrame) {
			f.ProviderExecutionDisposition = "accepted"
		}},
		{name: "unknown disposition", mutate: func(_ *aetherWSRouteControlConsumer, f *aetherWSRouteControlFrame) {
			f.ProviderExecutionDisposition = "unknown"
		}},
		{name: "confirmed without proof", mutate: func(_ *aetherWSRouteControlConsumer, f *aetherWSRouteControlFrame) {
			f.ProviderWriteState = "confirmed"
		}},
		{name: "arbitrary proof class", mutate: func(_ *aetherWSRouteControlConsumer, f *aetherWSRouteControlFrame) {
			f.CurrentAttemptState = "rejected_before_execution"
			f.ProviderWriteState = "confirmed"
			f.ProviderExecutionDisposition = "proven_not_executed"
			proofClass := "some_adapter_says_safe"
			proofVersion := AetherWSAdapterProofVersionV1
			f.AdapterProofClass = &proofClass
			f.AdapterProofVersion = &proofVersion
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			consumer := newAetherWSRouteControlTestConsumer(t, false)
			_, fence := prepareAetherWSRouteControlTestStep(t, consumer)
			frame := aetherWSRouteControlTestFrame(fence, aetherWSRouteActionClientReconnect)
			tt.mutate(consumer, &frame)
			consumed, _, err := consumer.consumeUpstreamFrame(marshalAetherWSRouteControlTestFrame(t, frame))
			require.True(t, consumed)
			require.Error(t, err)
		})
	}
}

func TestAetherWSRouteControl_ValidatesEveryLocalFenceDimension(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*aetherWSRouteControlFrame)
	}{
		{name: "version", mutate: func(f *aetherWSRouteControlFrame) { f.Version = 2 }},
		{name: "action", mutate: func(f *aetherWSRouteControlFrame) { f.Action = "switch_now" }},
		{name: "control id", mutate: func(f *aetherWSRouteControlFrame) { f.ControlID = "bad id" }},
		{name: "correlation", mutate: func(f *aetherWSRouteControlFrame) { f.Sub2APIStepCorrelationID = "other-correlation" }},
		{name: "binding epoch", mutate: func(f *aetherWSRouteControlFrame) { f.Sub2APIBindingEpochID = "other-binding" }},
		{name: "binding generation", mutate: func(f *aetherWSRouteControlFrame) { f.Sub2APIBindingGeneration++ }},
		{name: "aether step", mutate: func(f *aetherWSRouteControlFrame) { f.AetherStepID = "bad step" }},
		{name: "recommendation", mutate: func(f *aetherWSRouteControlFrame) { f.RecommendedAction = "close_after_terminal" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			consumer := newAetherWSRouteControlTestConsumer(t, false)
			_, fence := prepareAetherWSRouteControlTestStep(t, consumer)
			frame := aetherWSRouteControlTestFrame(fence, aetherWSRouteActionClientReconnect)
			tt.mutate(&frame)
			consumed, _, err := consumer.consumeUpstreamFrame(marshalAetherWSRouteControlTestFrame(t, frame))
			require.True(t, consumed)
			require.Error(t, err)
		})
	}
}

func TestAetherWSRouteControl_CloseAfterTerminalAndLateControl(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	_, fence := prepareAetherWSRouteControlTestStep(t, consumer)
	frame := aetherWSRouteControlTestFrame(fence, aetherWSRouteActionCloseAfterTerminal)

	consumed, decision, err := consumer.consumeUpstreamFrame(marshalAetherWSRouteControlTestFrame(t, frame))
	require.NoError(t, err)
	require.True(t, consumed)
	require.True(t, decision.CloseAfterTerminal)

	lateConsumer := newAetherWSRouteControlTestConsumer(t, false)
	_, lateFence := prepareAetherWSRouteControlTestStep(t, lateConsumer)
	lateConsumer.markProviderFrameWritten(true)
	lateFrame := aetherWSRouteControlTestFrame(lateFence, aetherWSRouteActionCloseAfterTerminal)
	consumed, _, err = lateConsumer.consumeUpstreamFrame(marshalAetherWSRouteControlTestFrame(t, lateFrame))
	require.True(t, consumed)
	require.ErrorContains(t, err, "after terminal commit")
}

func TestAetherWSRouteControl_RejectsProviderFallback(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	_, fence := prepareAetherWSRouteControlTestStep(t, consumer)
	frame := aetherWSRouteControlTestFrame(fence, aetherWSRouteActionClientReconnect)
	frame.ProviderFallbackUsed = true
	consumed, _, err := consumer.consumeUpstreamFrame(marshalAetherWSRouteControlTestFrame(t, frame))
	require.True(t, consumed)
	require.ErrorContains(t, err, "unsupported")
}

func TestAetherWSRouteControl_RejectsMalformedUnknownAndOversizedFrames(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	_, fence := prepareAetherWSRouteControlTestStep(t, consumer)
	valid := marshalAetherWSRouteControlTestFrame(t, aetherWSRouteControlTestFrame(fence, aetherWSRouteActionClientReconnect))

	unknown := append(valid[:len(valid)-1], []byte(`,"secret":"must-not-pass"}`)...)
	duplicate := []byte(`{"version":1,` + string(valid[1:]))
	trailing := append(append([]byte(nil), valid...), []byte(` {}`)...)
	oversized := []byte(`{"type":"aether.route_control","padding":"` + strings.Repeat("x", aetherWSRouteControlMaxFrameBytes) + `"}`)
	malformed := []byte(`{"type":"aether.route_control","version":1`)
	for name, payload := range map[string][]byte{
		"unknown":   unknown,
		"duplicate": duplicate,
		"trailing":  trailing,
		"oversized": oversized,
		"malformed": malformed,
	} {
		t.Run(name, func(t *testing.T) {
			consumed, _, err := consumer.consumeUpstreamFrame(payload)
			require.True(t, consumed, "reserved type must never pass through")
			require.Error(t, err)
		})
	}
}

func TestAetherWSRouteControl_ControlIDIsIdempotentButCannotBeReused(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	_, fence := prepareAetherWSRouteControlTestStep(t, consumer)
	frame := aetherWSRouteControlTestFrame(fence, aetherWSRouteActionCloseAfterTerminal)
	payload := marshalAetherWSRouteControlTestFrame(t, frame)

	consumed, first, err := consumer.consumeUpstreamFrame(payload)
	require.NoError(t, err)
	require.True(t, consumed)
	consumed, duplicate, err := consumer.consumeUpstreamFrame(payload)
	require.NoError(t, err)
	require.True(t, consumed)
	require.Equal(t, first, duplicate)

	frame.Reason = "different_reason"
	consumed, _, err = consumer.consumeUpstreamFrame(marshalAetherWSRouteControlTestFrame(t, frame))
	require.True(t, consumed)
	require.ErrorContains(t, err, "reused")
}

func TestAetherWSRouteControl_ControlIDCannotChangeMiddleRouteDisposition(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	_, fence := prepareAetherWSRouteControlTestStep(t, consumer)
	frame := aetherWSRouteControlTestFrame(fence, aetherWSRouteActionClientReconnect)
	payload := marshalAetherWSRouteControlTestFrame(t, frame)

	consumed, _, err := consumer.consumeUpstreamFrame(payload)
	require.NoError(t, err)
	require.True(t, consumed)

	exclude := OpenAIWSMiddleRouteDispositionExclude
	frame.MiddleRouteDisposition = &exclude
	consumed, _, err = consumer.consumeUpstreamFrame(marshalAetherWSRouteControlTestFrame(t, frame))
	require.True(t, consumed)
	require.ErrorContains(t, err, "reused")
}

func TestAetherWSRouteControl_ReconnectFeatureFlagOnlyGatesLaterMigration(t *testing.T) {
	consumer, err := newAetherWSRouteControlConsumer(aetherWSRouteControlConsumerConfig{
		Negotiated: AetherWSNegotiatedCapabilities{
			ControlProtocol:    AetherWSControlProtocolRouteV1,
			CloseAfterTerminal: true,
			ClientReconnect:    true,
		},
		ReconnectEnabled:    false,
		ReconnectSignalMode: "unset",
		BindingEpochID:      "binding-disabled",
	})
	require.NoError(t, err)
	_, fence := prepareAetherWSRouteControlTestStep(t, consumer)
	frame := aetherWSRouteControlTestFrame(fence, aetherWSRouteActionClientReconnect)
	consumed, decision, err := consumer.consumeUpstreamFrame(marshalAetherWSRouteControlTestFrame(t, frame))
	require.True(t, consumed)
	require.NoError(t, err)
	require.True(t, decision.InitialStepFailover)

	_, secondFence := prepareAetherWSRouteControlTestStep(t, consumer)
	secondFrame := aetherWSRouteControlTestFrame(secondFence, aetherWSRouteActionClientReconnect)
	secondFrame.ControlID = "control-2"
	consumed, _, err = consumer.consumeUpstreamFrame(marshalAetherWSRouteControlTestFrame(t, secondFrame))
	require.True(t, consumed)
	require.ErrorContains(t, err, "disabled")

	closeFrame := aetherWSRouteControlTestFrame(secondFence, aetherWSRouteActionCloseAfterTerminal)
	closeFrame.ControlID = "control-3"
	consumed, decision, err = consumer.consumeUpstreamFrame(marshalAetherWSRouteControlTestFrame(t, closeFrame))
	require.True(t, consumed)
	require.NoError(t, err)
	require.True(t, decision.CloseAfterTerminal)
}

func TestAetherWSReconnectErrorPayloadMatchesPinnedCodexEvent(t *testing.T) {
	payload := aetherWSReconnectErrorPayload()
	require.JSONEq(t, `{"type":"error","status":400,"error":{"type":"invalid_request_error","code":"websocket_connection_limit_reached","message":"Responses websocket connection limit reached (60 minutes). Create a new websocket connection to continue."}}`, string(payload))
}

func TestAetherWSRouteControl_ReconnectSignalModeIsNormalized(t *testing.T) {
	consumer, err := newAetherWSRouteControlConsumer(aetherWSRouteControlConsumerConfig{
		Negotiated: AetherWSNegotiatedCapabilities{
			ControlProtocol:    AetherWSControlProtocolRouteV1,
			CloseAfterTerminal: true,
			ClientReconnect:    true,
		},
		ReconnectEnabled:    true,
		ReconnectSignalMode: " WebSocket_Connection_Limit_Reached ",
		BindingEpochID:      "binding-normalized",
	})
	require.NoError(t, err)
	require.True(t, consumer.reconnectEnabled)
}

func TestAetherWSRouteControl_NonMatchHotPathHasZeroAllocations(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	payload := []byte(`{"type":"response.output_text.delta","delta":"ordinary provider output"}`)

	allocations := testing.AllocsPerRun(1_000, func() {
		consumed, _, err := consumer.consumeUpstreamFrame(payload)
		if consumed || err != nil {
			t.Fatalf("ordinary provider frame entered control path: consumed=%v err=%v", consumed, err)
		}
	})
	require.Zero(t, allocations)
}

func TestAetherWSRouteControl_LargeDeltaContainingMarkerDoesNotMaterializePayload(t *testing.T) {
	consumer := newAetherWSRouteControlTestConsumer(t, false)
	payload := []byte(`{"type":"response.output_text.delta","delta":"` + strings.Repeat("x", 1<<20) + `aether.route_control"}`)

	consumed, decision, err := consumer.consumeUpstreamFrame(payload)
	require.NoError(t, err)
	require.False(t, consumed)
	require.Equal(t, aetherWSRouteControlDecision{}, decision)
}

func BenchmarkAetherWSRouteControlLargeMarkerFalsePositive(b *testing.B) {
	consumer := &aetherWSRouteControlConsumer{enabled: true}
	payload := []byte(`{"type":"response.output_text.delta","delta":"` + strings.Repeat("x", 1<<20) + `aether.route_control"}`)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for range b.N {
		consumed, _, err := consumer.consumeUpstreamFrame(payload)
		if consumed || err != nil {
			b.Fatalf("ordinary provider frame entered control path: consumed=%v err=%v", consumed, err)
		}
	}
}

func BenchmarkAetherWSRouteControlNonMatch(b *testing.B) {
	consumer := &aetherWSRouteControlConsumer{enabled: true}
	for _, size := range []int{128, 4 << 10, 64 << 10, 1 << 20} {
		b.Run(strconv.Itoa(size)+"B", func(b *testing.B) {
			payload := bytes.Repeat([]byte{'x'}, size)
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			b.ResetTimer()
			for range b.N {
				consumed, _, err := consumer.consumeUpstreamFrame(payload)
				if consumed || err != nil {
					b.Fatalf("ordinary provider frame entered control path: consumed=%v err=%v", consumed, err)
				}
			}
		})
	}
}
