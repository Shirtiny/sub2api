package service

import (
	"testing"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestParseOpenAIWSClientEnvelopeRejectsEscapedDuplicateKey(t *testing.T) {
	t.Parallel()

	_, err := ParseOpenAIWSClientEnvelope([]byte(`{"type":"response.create","service_tier":"default","service_\u0074ier":"priority"}`))
	require.Error(t, err)
}

func TestValidateOpenAIWSPassthroughSessionModelFreezesPhysicalModel(t *testing.T) {
	t.Parallel()

	unchanged, err := ParseOpenAIWSClientEnvelope([]byte(`{"type":"session.update","session":{"model":"gpt-5"}}`))
	require.NoError(t, err)
	require.NoError(t, validateOpenAIWSPassthroughSessionModel(unchanged, "gpt-5"))

	changed, err := ParseOpenAIWSClientEnvelope([]byte(`{"type":"session.update","session":{"model":"gpt-5.1"}}`))
	require.NoError(t, err)
	freezeErr := validateOpenAIWSPassthroughSessionModel(changed, "gpt-5")
	require.Error(t, freezeErr)
	var closeErr *OpenAIWSClientCloseError
	require.ErrorAs(t, freezeErr, &closeErr)
	require.Equal(t, coderws.StatusPolicyViolation, closeErr.statusCode)
}

func TestOpenAIWSPassthroughSessionModelUsesClientDomainThenMapsUpstream(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"session.update","session":{"model":"client-model","voice":"alloy"}}`)
	envelope, err := ParseOpenAIWSClientEnvelope(payload)
	require.NoError(t, err)
	require.NoError(t, validateOpenAIWSPassthroughSessionModel(envelope, "client-model"))

	updated, err := transformOpenAIWSPassthroughSessionModel(payload, envelope, "upstream-model")
	require.NoError(t, err)
	require.Equal(t, "upstream-model", gjson.GetBytes(updated, "session.model").String())
	require.Equal(t, "alloy", gjson.GetBytes(updated, "session.voice").String())
}

func TestTransformOpenAIWSPassthroughResponseModelFreezesPhysicalModel(t *testing.T) {
	payload := []byte(`{"type":"response.create","model":"client-model","input":"keep"}`)
	updated, err := transformOpenAIWSPassthroughResponseModel(payload, "client-model", "provider-model")
	require.NoError(t, err)
	require.Equal(t, "provider-model", gjson.GetBytes(updated, "model").String())
	require.Equal(t, "keep", gjson.GetBytes(updated, "input").String())

	unchanged, err := transformOpenAIWSPassthroughResponseModel(updated, "provider-model", "provider-model")
	require.NoError(t, err)
	require.Equal(t, &updated[0], &unchanged[0], "an already-frozen payload must not be copied")
}

func TestAetherWSCombinedEditPreservesMetadataAndFreezesModel(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"response.create","model":"client-model","service_tier":"fast","client_metadata":{"session_id":"session-a","thread_id":"thread-a","nested":{"keep":true}},"input":[]}`)
	envelope, err := ParseOpenAIWSClientEnvelope(payload)
	require.NoError(t, err)
	consumer, err := newAetherWSRouteControlConsumer(aetherWSRouteControlConsumerConfig{
		Negotiated: AetherWSNegotiatedCapabilities{
			ControlProtocol:    AetherWSControlProtocolRouteV1,
			CloseAfterTerminal: true,
			ClientReconnect:    true,
		},
		BindingEpochID:    "combined-edit-epoch",
		BindingGeneration: 3,
	})
	require.NoError(t, err)

	updated, err := consumer.prepareValidatedResponseCreateWithEnvelopeAndModelAndServiceTier(
		payload,
		envelope,
		"provider-model",
		&aetherWSServiceTierMutation{remove: true},
	)
	require.NoError(t, err)
	require.Equal(t, "provider-model", gjson.GetBytes(updated, "model").String())
	require.False(t, gjson.GetBytes(updated, "service_tier").Exists())
	require.Equal(t, "session-a", gjson.GetBytes(updated, "client_metadata.session_id").String())
	require.True(t, gjson.GetBytes(updated, "client_metadata.nested.keep").Bool())
	require.Equal(t, "combined-edit-epoch", gjson.GetBytes(updated, "client_metadata.aether\\.sub2api_step_control.sub2api_binding_epoch_id").String())
	require.Equal(t, uint64(3), gjson.GetBytes(updated, "client_metadata.aether\\.sub2api_step_control.sub2api_binding_generation").Uint())

	updatedEnvelope, err := ParseOpenAIWSClientEnvelope(updated)
	require.NoError(t, err)
	require.True(t, updatedEnvelope.HasClientMetadata)
	require.True(t, updatedEnvelope.ClientMetadataIsObject)
	require.True(t, updatedEnvelope.HasAetherStepControl)
}
