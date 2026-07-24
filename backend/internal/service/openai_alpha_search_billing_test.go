package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func searchFloat64Ptr(value float64) *float64 { return &value }

func TestCalculateWebSearchCostDefaultOverrideAndFree(t *testing.T) {
	svc := &BillingService{}

	cost := svc.CalculateWebSearchCost(1, nil, 1)
	require.InDelta(t, 0.01, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.01, cost.ActualCost, 1e-12)
	require.Equal(t, string(BillingModePerRequest), cost.BillingMode)

	cost = svc.CalculateWebSearchCost(2, searchFloat64Ptr(0.02), 2.5)
	require.InDelta(t, 0.04, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.10, cost.ActualCost, 1e-12)

	cost = svc.CalculateWebSearchCost(1, searchFloat64Ptr(0), 3)
	require.Zero(t, cost.TotalCost)
	require.Zero(t, cost.ActualCost)

	cost = svc.CalculateWebSearchCost(0, nil, 1)
	require.Zero(t, cost.TotalCost)
	require.Empty(t, cost.BillingMode)
}

func TestCalculateOpenAIRecordUsageCostUsesWebSearchSurfaceOnly(t *testing.T) {
	svc := &OpenAIGatewayService{billingService: &BillingService{}}
	groupID := int64(11)
	apiKey := &APIKey{
		GroupID: &groupID,
		Group: &Group{
			ID:                    groupID,
			Platform:              PlatformOpenAI,
			WebSearchPricePerCall: searchFloat64Ptr(0.005),
		},
	}
	result := &OpenAIForwardResult{
		Model:          "unpriced-search-model",
		UpstreamModel:  "unpriced-search-model",
		WebSearchCalls: 1,
		Usage: OpenAIUsage{
			InputTokens:  999,
			OutputTokens: 999,
		},
	}

	cost, err := svc.calculateOpenAIRecordUsageCost(
		context.Background(), result, apiKey, []string{"unpriced-search-model"},
		2, 100, 100, UsageTokens{InputTokens: 999, OutputTokens: 999}, "",
	)
	require.NoError(t, err)
	require.Equal(t, string(BillingModePerRequest), cost.BillingMode)
	require.InDelta(t, 0.005, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.01, cost.ActualCost, 1e-12)
}

func TestRecordUsagePersistsAlphaSearchTimingAndPerCallBilling(t *testing.T) {
	groupID := int64(12)
	firstByteMs := 125
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{
		result: &UsageBillingApplyResult{Applied: true},
	}
	quotaSvc := &openAIRecordUsageAPIKeyQuotaStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(
		usageRepo,
		billingRepo,
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
		nil,
	)
	payloadHash := HashUsageRequestPayload([]byte(`{"model":"gpt-5.6-sol","commands":{"search_query":[{"q":"news"}]}}`))

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:      "search-usage",
			Model:          "gpt-5.6-sol",
			UpstreamModel:  "gpt-5.6-sol",
			Duration:       850 * time.Millisecond,
			FirstByteMs:    &firstByteMs,
			WebSearchCalls: 1,
		},
		APIKey: &APIKey{
			ID:      120,
			Quota:   10,
			GroupID: &groupID,
			Group: &Group{
				ID:                    groupID,
				Platform:              PlatformOpenAI,
				RateMultiplier:        2,
				WebSearchPricePerCall: searchFloat64Ptr(0.005),
			},
		},
		User:               &User{ID: 220},
		Account:            &Account{ID: 320, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		InboundEndpoint:    "/v1/alpha/search",
		UpstreamEndpoint:   "/v1/alpha/search",
		RequestPayloadHash: payloadHash,
		APIKeyService:      quotaSvc,
	})

	require.NoError(t, err)
	require.Equal(t, 1, billingRepo.calls)
	require.Equal(t, 1, usageRepo.calls)
	require.NotNil(t, usageRepo.lastLog)
	require.Zero(t, usageRepo.lastLog.InputTokens)
	require.Zero(t, usageRepo.lastLog.OutputTokens)
	require.Equal(t, RequestTypeSync, usageRepo.lastLog.EffectiveRequestType())
	require.NotNil(t, usageRepo.lastLog.DurationMs)
	require.Equal(t, 850, *usageRepo.lastLog.DurationMs)
	require.NotNil(t, usageRepo.lastLog.FirstByteMs)
	require.Equal(t, firstByteMs, *usageRepo.lastLog.FirstByteMs)
	require.Nil(t, usageRepo.lastLog.FirstTokenMs)
	require.NotNil(t, usageRepo.lastLog.BillingMode)
	require.Equal(t, string(BillingModePerRequest), *usageRepo.lastLog.BillingMode)
	require.InDelta(t, 0.005, usageRepo.lastLog.TotalCost, 1e-12)
	require.InDelta(t, 0.01, usageRepo.lastLog.ActualCost, 1e-12)
	require.NotNil(t, usageRepo.lastLog.InboundEndpoint)
	require.Equal(t, "/v1/alpha/search", *usageRepo.lastLog.InboundEndpoint)

	require.NotNil(t, billingRepo.lastCmd)
	require.Equal(t, "search-usage", billingRepo.lastCmd.RequestID)
	require.Equal(t, payloadHash, billingRepo.lastCmd.RequestPayloadHash)
	require.Zero(t, billingRepo.lastCmd.InputTokens)
	require.Zero(t, billingRepo.lastCmd.OutputTokens)
	require.InDelta(t, 0.01, billingRepo.lastCmd.BalanceCost, 1e-12)
	require.InDelta(t, 0.01, billingRepo.lastCmd.APIKeyQuotaCost, 1e-12)
}
