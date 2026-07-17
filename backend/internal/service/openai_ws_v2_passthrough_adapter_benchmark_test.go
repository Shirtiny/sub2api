package service

import (
	"bytes"
	"context"
	"fmt"
	"testing"
)

var openAIWSPipelineBenchmarkSink int
var openAIWSPipelineBenchmarkString string

func BenchmarkOpenAIWSPassthroughBusinessFramePipeline(b *testing.B) {
	for _, size := range []int{64 * 1024, 1024 * 1024} {
		b.Run(fmt.Sprintf("%dKiB", size/1024), func(b *testing.B) {
			prefix := []byte(`{"type":"response.create","model":"client-model","client_metadata":{"session_id":"sess-bench","thread_id":"thread-bench","origin":"codex"},"input":"`)
			suffix := []byte(`"}`)
			payload := make([]byte, 0, size)
			payload = append(payload, prefix...)
			payload = append(payload, bytes.Repeat([]byte{'x'}, size-len(prefix)-len(suffix))...)
			payload = append(payload, suffix...)
			account := &Account{
				Type: AccountTypeAPIKey,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"channel-model": "provider-model"},
				},
			}
			consumer, err := newAetherWSRouteControlConsumer(aetherWSRouteControlConsumerConfig{
				Negotiated: AetherWSNegotiatedCapabilities{
					ControlProtocol:    AetherWSControlProtocolRouteV1,
					CloseAfterTerminal: true,
					ClientReconnect:    true,
				},
				BindingEpochID:    "benchmark-binding-epoch",
				BindingGeneration: 1,
			})
			if err != nil {
				b.Fatal(err)
			}
			svc := &OpenAIGatewayService{}
			ctx := context.Background()
			usageMeta := newOpenAIWSPassthroughUsageMetaFromEnvelope("client-model", OpenAIWSClientEnvelope{})

			b.SetBytes(int64(len(payload)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				envelope, err := ParseOpenAIWSClientEnvelope(payload)
				if err != nil {
					b.Fatal(err)
				}
				_ = HashUsageRequestPayload(payload)
				physicalModel := normalizeOpenAIModelForUpstream(account, account.GetMappedModel("channel-model"))
				filtered, blocked, err := svc.applyValidatedOpenAIFastPolicyToWSResponseCreateWithTier(
					ctx, account, physicalModel, payload, envelope.ServiceTier, envelope.HasServiceTier,
				)
				if err != nil || blocked != nil {
					b.Fatalf("policy: blocked=%v err=%v", blocked, err)
				}
				usageMeta.updateFromEnvelope(envelope, envelope.Model)
				routed, err := consumer.prepareValidatedResponseCreateWithEnvelopeAndModel(filtered, envelope, physicalModel)
				if err != nil {
					b.Fatal(err)
				}
				openAIWSPipelineBenchmarkSink = len(routed)
			}
		})
	}
}

func BenchmarkOpenAIWSPassthroughBusinessFramePipelinePolicyFiltered(b *testing.B) {
	for _, size := range []int{64 * 1024, 1024 * 1024} {
		b.Run(fmt.Sprintf("%dKiB", size/1024), func(b *testing.B) {
			prefix := []byte(`{"type":"response.create","model":"client-model","service_tier":"fast","client_metadata":{"session_id":"sess-bench","thread_id":"thread-bench","origin":"codex"},"input":"`)
			suffix := []byte(`"}`)
			payload := make([]byte, 0, size)
			payload = append(payload, prefix...)
			payload = append(payload, bytes.Repeat([]byte{'x'}, size-len(prefix)-len(suffix))...)
			payload = append(payload, suffix...)
			account := &Account{
				Type: AccountTypeAPIKey,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"channel-model": "provider-model"},
				},
			}
			consumer, err := newAetherWSRouteControlConsumer(aetherWSRouteControlConsumerConfig{
				Negotiated: AetherWSNegotiatedCapabilities{
					ControlProtocol:    AetherWSControlProtocolRouteV1,
					CloseAfterTerminal: true,
					ClientReconnect:    true,
				},
				BindingEpochID:    "benchmark-binding-epoch",
				BindingGeneration: 1,
			})
			if err != nil {
				b.Fatal(err)
			}
			svc := &OpenAIGatewayService{}
			ctx := withOpenAIFastPolicyContext(context.Background(), &OpenAIFastPolicySettings{
				Rules: []OpenAIFastPolicyRule{{
					ServiceTier: OpenAIFastTierPriority,
					Action:      BetaPolicyActionFilter,
					Scope:       BetaPolicyScopeAll,
				}},
			})
			usageMeta := newOpenAIWSPassthroughUsageMetaFromEnvelope("client-model", OpenAIWSClientEnvelope{})

			b.SetBytes(int64(len(payload)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				envelope, err := ParseOpenAIWSClientEnvelope(payload)
				if err != nil {
					b.Fatal(err)
				}
				_ = HashUsageRequestPayload(payload)
				physicalModel := normalizeOpenAIModelForUpstream(account, account.GetMappedModel("channel-model"))
				decision, blocked := svc.evaluateOpenAIFastPolicyForWS(
					ctx, account, physicalModel, envelope.ServiceTier, envelope.HasServiceTier,
				)
				if blocked != nil {
					b.Fatalf("policy blocked: %v", blocked)
				}
				usageMeta.updateFromEnvelope(decision.applyToEnvelope(envelope), envelope.Model)
				routed, err := consumer.prepareValidatedResponseCreateWithEnvelopeAndModelAndServiceTier(payload, envelope, physicalModel, decision.serviceTierMutation())
				if err != nil {
					b.Fatal(err)
				}
				openAIWSPipelineBenchmarkSink = len(routed)
			}
		})
	}
}

func BenchmarkOpenAIWSPassthroughBusinessFrameComponents1MiB(b *testing.B) {
	const size = 1024 * 1024
	prefix := []byte(`{"type":"response.create","model":"client-model","client_metadata":{"session_id":"sess-bench","thread_id":"thread-bench","origin":"codex"},"input":"`)
	suffix := []byte(`"}`)
	payload := make([]byte, 0, size)
	payload = append(payload, prefix...)
	payload = append(payload, bytes.Repeat([]byte{'x'}, size-len(prefix)-len(suffix))...)
	payload = append(payload, suffix...)
	account := &Account{Type: AccountTypeAPIKey}
	svc := &OpenAIGatewayService{}
	ctx := context.Background()

	b.Run("parse", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			envelope, err := ParseOpenAIWSClientEnvelope(payload)
			if err != nil {
				b.Fatal(err)
			}
			openAIWSPipelineBenchmarkString = envelope.Model
		}
	})
	b.Run("hash", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			openAIWSPipelineBenchmarkString = HashUsageRequestPayload(payload)
		}
	})
	b.Run("policy_no_tier", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			out, blocked, err := svc.applyValidatedOpenAIFastPolicyToWSResponseCreate(ctx, account, "provider-model", payload)
			if err != nil || blocked != nil {
				b.Fatal(err)
			}
			openAIWSPipelineBenchmarkSink = len(out)
		}
	})
	b.Run("model_rewrite", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			out, err := transformOpenAIWSPassthroughResponseModel(payload, "client-model", "provider-model")
			if err != nil {
				b.Fatal(err)
			}
			openAIWSPipelineBenchmarkSink = len(out)
		}
	})
	b.Run("route_fence", func(b *testing.B) {
		envelope, err := ParseOpenAIWSClientEnvelope(payload)
		if err != nil {
			b.Fatal(err)
		}
		consumer, err := newAetherWSRouteControlConsumer(aetherWSRouteControlConsumerConfig{
			Negotiated:     AetherWSNegotiatedCapabilities{ControlProtocol: AetherWSControlProtocolRouteV1, CloseAfterTerminal: true, ClientReconnect: true},
			BindingEpochID: "benchmark-binding-epoch", BindingGeneration: 1,
		})
		if err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			out, err := consumer.prepareValidatedResponseCreateWithEnvelope(payload, envelope)
			if err != nil {
				b.Fatal(err)
			}
			openAIWSPipelineBenchmarkSink = len(out)
		}
	})
}
