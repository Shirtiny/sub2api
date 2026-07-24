package service

import (
	"context"
	"testing"

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
