package openai_ws_v2

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseClientEnvelopeExtractsBoundedMetadata(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"ty\u0070e":"response.create","model":" gpt-5.3-codex ","previous_response_id":" resp_123 ","service_tier":"priority","input":[{"type":"input_text","text":"hello"}]}`)
	envelope, err := ParseClientEnvelope(payload)
	require.NoError(t, err)
	require.Equal(t, "response.create", envelope.Type)
	require.Equal(t, "gpt-5.3-codex", envelope.Model)
	require.True(t, envelope.HasModel)
	require.Equal(t, "resp_123", envelope.PreviousResponseID)
	require.True(t, envelope.HasPreviousResponseID)
	require.Equal(t, "priority", envelope.ServiceTier)
	require.True(t, envelope.HasServiceTier)
}

func TestClientEnvelopeServiceTierRangesSupportDeletion(t *testing.T) {
	t.Parallel()

	for name, payload := range map[string][]byte{
		"first":  []byte(`{"service_tier":"fast", "type":"response.create","model":"gpt-5"}`),
		"middle": []byte(`{"type":"response.create", "service_tier":"fast", "model":"gpt-5"}`),
		"last":   []byte(`{"type":"response.create","model":"gpt-5", "service_tier":"fast" }`),
	} {
		name, payload := name, payload
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			envelope, err := ParseClientEnvelope(payload)
			require.NoError(t, err)
			valueStart, valueEnd, ok := envelope.ServiceTierValueRange()
			require.True(t, ok)
			require.JSONEq(t, `"fast"`, string(payload[valueStart:valueEnd]))
			fieldStart, fieldEnd, ok := envelope.ServiceTierFieldRange()
			require.True(t, ok)
			withoutTier := append(append([]byte(nil), payload[:fieldStart]...), payload[fieldEnd:]...)
			reparsed, err := ParseClientEnvelope(withoutTier)
			require.NoError(t, err)
			require.False(t, reparsed.HasServiceTier)
			require.Equal(t, "response.create", reparsed.Type)
		})
	}
}

func TestParseClientEnvelopeExtractsClientMetadataRouteIdentity(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"response.create","model":"client-model","reasoning":{"effort":"high"},"prompt_cache_key":"cache-a","input":"hello","client_metadata":{"session_id":" session-a ","thread_id":" thread-a ","future":{"keep":true},"aether.sub2api_step_control":{"spoof":true}}}`)
	envelope, err := ParseClientEnvelope(payload)
	require.NoError(t, err)
	require.True(t, envelope.HasClientMetadataRouteIdentity)
	require.Equal(t, "session-a", envelope.ClientMetadataSessionID)
	require.Equal(t, "thread-a", envelope.ClientMetadataThreadID)
	require.True(t, envelope.HasClientMetadata)
	require.True(t, envelope.ClientMetadataIsObject)
	require.True(t, envelope.HasAetherStepControl)
	require.Equal(t, "high", envelope.ReasoningEffort)
	require.True(t, envelope.HasReasoningEffort)
	require.Equal(t, "cache-a", envelope.PromptCacheKey)
	require.True(t, envelope.HasPromptCacheKey)
	modelStart, modelEnd, ok := envelope.ModelValueRange()
	require.True(t, ok)
	require.Equal(t, `"client-model"`, string(payload[modelStart:modelEnd]))
	metadataStart, metadataEnd, ok := envelope.ClientMetadataValueRange()
	require.True(t, ok)
	require.JSONEq(t, `{"session_id":" session-a ","thread_id":" thread-a ","future":{"keep":true},"aether.sub2api_step_control":{"spoof":true}}`, string(payload[metadataStart:metadataEnd]))

	envelope, err = ParseClientEnvelope([]byte(`{"type":"response.create","client_metadata":null}`))
	require.NoError(t, err)
	require.False(t, envelope.HasClientMetadataRouteIdentity)
}

func TestParseClientEnvelopeRejectsEveryDuplicateTopLevelKey(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"type literal":       `{"type":"response.create","type":"response.create"}`,
		"type escaped alias": `{"type":"response.create","ty\u0070e":"response.create"}`,
		"model":              `{"type":"response.create","model":"a","mo\u0064el":"b"}`,
		"service tier":       `{"type":"response.create","service_tier":"default","service_\u0074ier":"priority"}`,
	}
	for name, payload := range tests {
		name, payload := name, payload
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseClientEnvelope([]byte(payload))
			require.ErrorIs(t, err, errClientEnvelopeDuplicateField)
		})
	}
}

func TestParseClientEnvelopeRejectsInvalidMetadataTypesAndValues(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"type number":            `{"type":1}`,
		"model null":             `{"type":"response.create","model":null}`,
		"previous response bool": `{"type":"response.create","previous_response_id":true}`,
		"non ascii model":        `{"type":"response.create","model":"模型"}`,
		"non ascii response id":  `{"type":"response.create","previous_response_id":"响应"}`,
		"session model number":   `{"type":"session.update","session":{"model":1}}`,
		"client metadata number": `{"type":"response.create","client_metadata":1}`,
		"partial route identity": `{"type":"response.create","client_metadata":{"session_id":"session-a"}}`,
		"duplicate route id":     `{"type":"response.create","client_metadata":{"session_id":"session-a","session_\u0069d":"session-b","thread_id":"thread-a"}}`,
		"route id too long":      `{"type":"response.create","client_metadata":{"session_id":"` + strings.Repeat("x", ClientEnvelopeMaxRouteIDBytes+1) + `","thread_id":"thread-a"}}`,
	}
	for name, payload := range tests {
		name, payload := name, payload
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseClientEnvelope([]byte(payload))
			require.Error(t, err)
		})
	}
}

func TestParseClientEnvelopeStructuralLimits(t *testing.T) {
	t.Parallel()

	t.Run("event type", func(t *testing.T) {
		_, err := ParseClientEnvelope([]byte(`{"type":"` + strings.Repeat("x", ClientEnvelopeMaxEventTypeBytes+1) + `"}`))
		require.Error(t, err)
	})
	t.Run("identifier", func(t *testing.T) {
		_, err := ParseClientEnvelope([]byte(`{"type":"response.create","model":"` + strings.Repeat("x", ClientEnvelopeMaxIdentifierBytes+1) + `"}`))
		require.Error(t, err)
	})
	t.Run("key", func(t *testing.T) {
		_, err := ParseClientEnvelope([]byte(`{"type":"response.create","` + strings.Repeat("x", ClientEnvelopeMaxKeyBytes+1) + `":1}`))
		require.ErrorIs(t, err, errClientEnvelopeKeyTooLong)
	})
	t.Run("field count", func(t *testing.T) {
		var payload strings.Builder
		_, err := payload.WriteString(`{"type":"response.create"`)
		require.NoError(t, err)
		for index := 0; index < ClientEnvelopeMaxTopLevelFields; index++ {
			fmt.Fprintf(&payload, `,"f%d":%d`, index, index)
		}
		err = payload.WriteByte('}')
		require.NoError(t, err)
		_, err = ParseClientEnvelope([]byte(payload.String()))
		require.ErrorIs(t, err, errClientEnvelopeTooManyFields)
	})
	t.Run("depth", func(t *testing.T) {
		payload := `{"type":"response.create","input":` + strings.Repeat("[", ClientEnvelopeMaxDepth+1) + `0` + strings.Repeat("]", ClientEnvelopeMaxDepth+1) + `}`
		_, err := ParseClientEnvelope([]byte(payload))
		require.ErrorIs(t, err, errClientEnvelopeTooDeep)
	})
}

func TestParseClientEnvelopeExtractsAndDeduplicatesSessionModel(t *testing.T) {
	t.Parallel()

	envelope, err := ParseClientEnvelope([]byte(`{"type":"session.update","session":{"voice":"alloy","model":"gpt-5"}}`))
	require.NoError(t, err)
	require.True(t, envelope.HasSessionModel)
	require.Equal(t, "gpt-5", envelope.SessionModel)

	_, err = ParseClientEnvelope([]byte(`{"type":"session.update","session":{"model":"gpt-5","mo\u0064el":"gpt-5.1"}}`))
	require.ErrorIs(t, err, errClientEnvelopeDuplicateField)
}

func TestParseClientEnvelopeDoesNotCopyRequestBody(t *testing.T) {
	small := []byte(`{"type":"response.create","model":"gpt-5","input":"x","client_metadata":{"session_id":"session-a","thread_id":"thread-a"}}`)
	large := []byte(`{"type":"response.create","model":"gpt-5","input":"` + strings.Repeat("x", 1024*1024) + `","client_metadata":{"session_id":"session-a","thread_id":"thread-a"}}`)

	smallAllocs := testing.AllocsPerRun(50, func() {
		_, err := ParseClientEnvelope(small)
		if err != nil {
			panic(err)
		}
	})
	largeAllocs := testing.AllocsPerRun(50, func() {
		_, err := ParseClientEnvelope(large)
		if err != nil {
			panic(err)
		}
	})
	require.LessOrEqual(t, largeAllocs, smallAllocs+1, "allocation count must not grow with payload size")
}

func TestInspectTopLevelStringFieldHandlesEscapesDuplicatesAndPartialErrors(t *testing.T) {
	t.Parallel()

	inspection, err := InspectTopLevelStringField(
		[]byte(`{"ty\u0070e":"aether.route_\u0063ontrol","type":"ordinary","nested":{"type":"aether.route_control"}}`),
		"type",
		"aether.route_control",
	)
	require.NoError(t, err)
	require.Equal(t, 2, inspection.Count)
	require.True(t, inspection.Matched)

	inspection, err = InspectTopLevelStringField(
		[]byte(`{"type":"aether.route_control","type":1,"broken":`),
		"type",
		"aether.route_control",
	)
	require.Error(t, err)
	require.Equal(t, 2, inspection.Count)
	require.True(t, inspection.Matched, "a later parse error must not erase the reserved-type classification")
}

func TestInspectTopLevelStringFieldDoesNotMaterializeUnknownLargeString(t *testing.T) {
	payload := []byte(`{"type":"response.output_text.delta","delta":"` + strings.Repeat("x", 1024*1024) + `aether.route_control"}`)

	inspection, err := InspectTopLevelStringField(payload, "type", "aether.route_control")
	require.NoError(t, err)
	require.Equal(t, 1, inspection.Count)
	require.False(t, inspection.Matched)

	allocs := testing.AllocsPerRun(50, func() {
		result, inspectErr := InspectTopLevelStringField(payload, "type", "aether.route_control")
		if inspectErr != nil || result.Count != 1 || result.Matched {
			panic("unexpected inspection result")
		}
	})
	require.Zero(t, allocs)
}

func TestInspectFirstTopLevelStringFieldIsZeroAllocationPrefixHint(t *testing.T) {
	payload := []byte(`{"type":"aether.route_control","delta":"` + strings.Repeat("x", 1024*1024))

	inspection, err := InspectFirstTopLevelStringField(payload, "type", "aether.route_control")
	require.NoError(t, err, "the prefix hint intentionally ignores malformed trailing bytes")
	require.Equal(t, 1, inspection.Count)
	require.True(t, inspection.Matched)

	allocs := testing.AllocsPerRun(1_000, func() {
		result, inspectErr := InspectFirstTopLevelStringField(payload, "type", "aether.route_control")
		if inspectErr != nil || result.Count != 1 || !result.Matched {
			panic("unexpected first-field inspection result")
		}
	})
	require.Zero(t, allocs)
}

func BenchmarkParseClientEnvelopeLargePayload(b *testing.B) {
	payload := []byte(`{"type":"response.create","model":"gpt-5","input":"` + strings.Repeat("x", 1024*1024) + `","client_metadata":{"session_id":"session-a","thread_id":"thread-a"}}`)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := ParseClientEnvelope(payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInspectTopLevelStringFieldLargePayload(b *testing.B) {
	payload := []byte(`{"type":"response.output_text.delta","delta":"` + strings.Repeat("x", 1024*1024) + `aether.route_control"}`)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := InspectTopLevelStringField(payload, "type", "aether.route_control"); err != nil {
			b.Fatal(err)
		}
	}
}
