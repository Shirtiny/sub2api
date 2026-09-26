//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelPriceMultiplierRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := &channelRepository{db: integrationDB}
	factor, price := .1, .000002
	var groupID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "INSERT INTO groups (name, platform) VALUES ($1, 'openai') RETURNING id", fmt.Sprintf("multiplier-group-%d", time.Now().UnixNano())).Scan(&groupID))
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(ctx, "DELETE FROM groups WHERE id=$1", groupID) })
	channel := &service.Channel{Name: fmt.Sprintf("multiplier-%d", time.Now().UnixNano()), Status: service.StatusActive,
		ModelPricing:             []service.ChannelModelPricing{{Platform: "openai", Models: []string{"example-model"}, PriceMultiplier: &factor, InputPrice: &price}},
		AccountStatsPricingRules: []service.AccountStatsPricingRule{{Name: "cost", GroupIDs: []int64{groupID}, AccountIDs: []int64{}, Pricing: []service.ChannelModelPricing{{Platform: "openai", Models: []string{"example-model"}, PriceMultiplier: &factor, InputPrice: &price}}}},
	}
	require.NoError(t, repo.Create(ctx, channel))
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(ctx, "DELETE FROM channels WHERE id=$1", channel.ID) })
	saved, err := repo.GetByID(ctx, channel.ID)
	require.NoError(t, err)
	require.Equal(t, .1, saved.ModelPricing[0].EffectivePriceMultiplier())
	require.Equal(t, price, *saved.ModelPricing[0].InputPrice)
	require.Equal(t, .1, saved.AccountStatsPricingRules[0].Pricing[0].EffectivePriceMultiplier())
	factor = 5
	saved.ModelPricing[0].PriceMultiplier = &factor
	require.NoError(t, repo.UpdateModelPricing(ctx, &saved.ModelPricing[0]))
	rows, err := repo.ListModelPricing(ctx, channel.ID)
	require.NoError(t, err)
	require.Equal(t, 5.0, rows[0].EffectivePriceMultiplier())
	require.Equal(t, price, *rows[0].InputPrice)
	require.NoError(t, repo.ReplaceModelPricing(ctx, channel.ID, []service.ChannelModelPricing{{Models: []string{"legacy"}, InputPrice: &price}}))
	rows, err = repo.ListModelPricing(ctx, channel.ID)
	require.NoError(t, err)
	require.Equal(t, 1.0, rows[0].EffectivePriceMultiplier())
	for _, invalid := range []string{"0", "-1", "1001", "'NaN'", "'Infinity'"} {
		_, err = integrationDB.ExecContext(ctx, "UPDATE channel_model_pricing SET price_multiplier="+invalid+" WHERE channel_id=$1", channel.ID)
		require.Error(t, err)
	}
}
