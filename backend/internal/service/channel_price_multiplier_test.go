//go:build unit

package service

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelPriceMultiplierReachesUsageAndDeduction(t *testing.T) {
	for _, factor := range []float64{.1, 5} {
		for _, subscription := range []bool{false, true} {
			for _, tiered := range []bool{false, true} {
				t.Run(fmt.Sprintf("factor=%g/subscription=%t/tiered=%t", factor, subscription, tiered), func(t *testing.T) {
					usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
					billingRepo := &openAIRecordUsageBillingRepoStub{}
					userRepo := &openAIRecordUsageUserRepoStub{}
					subRepo := &openAIRecordUsageSubRepoStub{}
					userRate := .4
					svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, &openAIUserGroupRateRepoStub{rate: &userRate})
					p := ChannelModelPricing{Models: []string{"gpt-5.1"}, Platform: "openai", BillingMode: BillingModeToken,
						PriceMultiplier: &factor, InputPrice: testPtrFloat64(2e-6), OutputPrice: testPtrFloat64(10e-6),
						CacheReadPrice: testPtrFloat64(.2e-6), CacheWritePrice: testPtrFloat64(2.5e-6)}
					originalCost := (700*2 + 80*10 + 200*.2 + 100*2.5) * 1e-6
					if tiered {
						maxTokens := 1500
						p.Intervals = []PricingInterval{{MinTokens: 500, MaxTokens: &maxTokens,
							InputPrice: testPtrFloat64(4e-6), OutputPrice: testPtrFloat64(20e-6),
							CacheReadPrice: testPtrFloat64(.4e-6), CacheWritePrice: testPtrFloat64(5e-6)}}
						originalCost *= 2
					}
					channel := Channel{ID: 1, Status: StatusActive, GroupIDs: []int64{10}, ModelPricing: []ChannelModelPricing{p}}
					svc.resolver = NewModelPricingResolver(newTestChannelService(makeStandardRepo(channel, map[int64]string{10: "openai"})), svc.billingService)
					input := &OpenAIRecordUsageInput{
						Result: &OpenAIForwardResult{RequestID: "multiplier-check", Model: "gpt-5.1", OpenAIWSMode: subscription,
							Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 80, CacheReadInputTokens: 200, CacheCreationInputTokens: 100}},
						APIKey: &APIKey{ID: 101, GroupID: i64p(10), Quota: 100, Group: &Group{ID: 10, RateMultiplier: .8}},
						User:   &User{ID: 201}, Account: &Account{ID: 301}, APIKeyService: &openAIRecordUsageAPIKeyQuotaStub{},
					}
					if subscription {
						input.APIKey.Group.SubscriptionType = SubscriptionTypeSubscription
						input.Subscription = &UserSubscription{ID: 401}
					}
					require.NoError(t, svc.RecordUsage(context.Background(), input))
					expected := originalCost * factor * userRate
					require.NotNil(t, usageRepo.lastLog)
					require.InDelta(t, originalCost*factor, usageRepo.lastLog.TotalCost, 1e-12)
					require.InDelta(t, expected, usageRepo.lastLog.ActualCost, 1e-12)
					require.Equal(t, 1, billingRepo.calls)
					require.InDelta(t, expected, billingRepo.lastCmd.APIKeyQuotaCost, 1e-12)
					if subscription {
						require.InDelta(t, expected, billingRepo.lastCmd.SubscriptionCost, 1e-12)
						require.Zero(t, billingRepo.lastCmd.BalanceCost)
					} else {
						require.InDelta(t, expected, billingRepo.lastCmd.BalanceCost, 1e-12)
						require.Zero(t, billingRepo.lastCmd.SubscriptionCost)
					}
					require.Zero(t, userRepo.deductCalls)
					require.Zero(t, subRepo.incrementCalls)
				})
			}
		}
	}
}

func TestChannelPriceMultiplierValidation(t *testing.T) {
	for _, value := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1), 1001} {
		require.Error(t, validatePricingEntries([]ChannelModelPricing{{PriceMultiplier: &value}}))
	}
	for _, value := range []*float64{nil, testPtrFloat64(.1), testPtrFloat64(5), testPtrFloat64(1000)} {
		require.NoError(t, validatePricingEntries([]ChannelModelPricing{{PriceMultiplier: value}}))
	}
}

func TestChannelPriceMultiplierBilling(t *testing.T) {
	// Cover flat, context tiers, image/request tiers, fallback fields, priority,
	// cache and group rates. Reusing the same resolved pricing must not compound it.
	for _, mode := range []BillingMode{BillingModeToken, BillingModePerRequest, BillingModeImage} {
		for _, multiplier := range []float64{.1, 1, 5} {
			for _, tiered := range []bool{false, true} {
				p := ChannelModelPricing{Models: []string{"claude-sonnet-4"}, Platform: "anthropic", BillingMode: mode,
					InputPrice: testPtrFloat64(8e-6), OutputPrice: testPtrFloat64(20e-6),
					CacheReadPrice: testPtrFloat64(.5e-6), CacheWritePrice: testPtrFloat64(10e-6),
					ImageOutputPrice: testPtrFloat64(2e-6), PerRequestPrice: testPtrFloat64(2),
				}
				if tiered {
					p.Intervals = []PricingInterval{{MinTokens: 0, MaxTokens: nil, TierLabel: "HD",
						InputPrice: testPtrFloat64(10e-6), OutputPrice: testPtrFloat64(30e-6),
						CacheReadPrice: testPtrFloat64(1e-6), CacheWritePrice: testPtrFloat64(12e-6), PerRequestPrice: testPtrFloat64(4)}}
				}
				bs := newTestBillingServiceForResolver()
				makeResolver := func(pricing ChannelModelPricing) *ModelPricingResolver {
					ch := Channel{ID: 1, Status: StatusActive, GroupIDs: []int64{10}, ModelPricing: []ChannelModelPricing{pricing}}
					return NewModelPricingResolver(newTestChannelService(makeStandardRepo(ch, map[int64]string{10: "anthropic"})), bs)
				}
				groupID := int64(10)
				input := CostInput{Ctx: context.Background(), GroupID: &groupID, Model: "claude-sonnet-4",
					Tokens:       UsageTokens{InputTokens: 100, OutputTokens: 80, CacheReadTokens: 50, CacheCreationTokens: 20, ImageOutputTokens: 10},
					RequestCount: 3, SizeTier: "HD", RateMultiplier: .4, ServiceTier: "priority", Resolver: makeResolver(p)}
				original, err := bs.CalculateCostUnified(input)
				require.NoError(t, err)
				p.PriceMultiplier = &multiplier
				input.Resolver = makeResolver(p)
				input.Resolved = input.Resolver.Resolve(input.Ctx, PricingInput{Model: input.Model, GroupID: &groupID})
				for attempt := 0; attempt < 3; attempt++ {
					got, err := bs.CalculateCostUnified(input)
					require.NoError(t, err)
					require.InDelta(t, original.TotalCost*multiplier, got.TotalCost, 1e-12)
					require.InDelta(t, got.TotalCost*.4, got.ActualCost, 1e-12)
					require.InDelta(t, original.InputCost*multiplier, got.InputCost, 1e-12)
					require.InDelta(t, original.OutputCost*multiplier, got.OutputCost, 1e-12)
					require.InDelta(t, original.CacheReadCost*multiplier, got.CacheReadCost, 1e-12)
					require.InDelta(t, original.CacheCreationCost*multiplier, got.CacheCreationCost, 1e-12)
					require.InDelta(t, original.ImageOutputCost*multiplier, got.ImageOutputCost, 1e-12)
				}
				require.InDelta(t, 8e-6, *p.InputPrice, 1e-12)
				require.InDelta(t, 3e-6, bs.fallbackPrices["claude-sonnet-4"].InputPricePerToken, 1e-12)
			}
		}
	}
}

func TestChannelPriceMultiplierFallbackAndStats(t *testing.T) {
	p := &ChannelModelPricing{PriceMultiplier: testPtrFloat64(.1)}
	got := synthesizePricingFromLiteLLM(&LiteLLMModelPricing{InputCostPerToken: 3e-6}, p)
	require.Equal(t, .1, got.EffectivePriceMultiplier())
	require.InDelta(t, 3e-6, *got.InputPrice, 1e-12) // DTO retains original prices
	bs := newTestBillingServiceForResolver()
	for i := 0; i < 2; i++ {
		prices, err := bs.GetModelPricingWithChannel("claude-sonnet-4", p)
		require.NoError(t, err)
		require.InDelta(t, .3e-6, prices.InputPricePerToken, 1e-12)
	}
	require.InDelta(t, 3e-6, bs.fallbackPrices["claude-sonnet-4"].InputPricePerToken, 1e-12)
	p.InputPrice = testPtrFloat64(2e-6)
	cost := calculateStatsCost(p, UsageTokens{InputTokens: 1000}, 1)
	require.NotNil(t, cost)
	require.InDelta(t, .0002, *cost, 1e-12)
}

func TestChannelPriceMultiplierUsesConfiguredFlatFallbackOutsideIntervals(t *testing.T) {
	bs := newTestBillingServiceForResolver()
	resolver := NewModelPricingResolver(nil, bs)
	p := &ChannelModelPricing{InputPrice: testPtrFloat64(8e-6), OutputPrice: testPtrFloat64(20e-6), PriceMultiplier: testPtrFloat64(.1),
		Intervals: []PricingInterval{{MinTokens: 1000, MaxTokens: nil, InputPrice: testPtrFloat64(10e-6)}}}
	resolved := &ResolvedPricing{Mode: BillingModeToken, BasePricing: bs.fallbackPrices["claude-sonnet-4"], channelPricing: p}
	resolver.applyTokenOverrides(p, resolved)
	got, err := bs.CalculateCostUnified(CostInput{Model: "claude-sonnet-4", Tokens: UsageTokens{InputTokens: 100}, Resolver: resolver, Resolved: resolved, RateMultiplier: .5})
	require.NoError(t, err)
	require.InDelta(t, 100*8e-6*.1, got.TotalCost, 1e-12)
	require.InDelta(t, got.TotalCost*.5, got.ActualCost, 1e-12)
	require.InDelta(t, 3e-6, bs.fallbackPrices["claude-sonnet-4"].InputPricePerToken, 1e-12)
}
