//go:build unit

package service

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestAdminOrderSummaryUsesPurchaseSnapshotWithoutProviderSecrets(t *testing.T) {
	s, _, p := newPresaleFixture(t)
	ctx := context.Background()
	provider := s.entClient.PaymentProviderInstance.Create().SetProviderKey(payment.TypeStripe).SetName("Current Stripe channel").SetConfig(`{"secret_key":"do-not-expose"}`).SaveX(ctx)
	providerID := strconv.FormatInt(provider.ID, 10)
	o := &dbent.PaymentOrder{OrderType: payment.OrderTypeSubscription, PlanID: &p.ID, PresalePlanName: "Original purchased plan", SubscriptionSourceGroupID: &p.GroupID, ProviderInstanceID: &providerID,
		ProviderSnapshot: map[string]any{"schema_version": 2, "currency": "USD", "payment_mode": "embedded", "merchant_id": "private-merchant", "secret_key": "secret"}}
	s.entClient.SubscriptionPlan.UpdateOneID(p.ID).SetName("Renamed plan").SetPrice(999).ExecX(ctx)
	summary, err := s.GetAdminOrderSummary(ctx, o)
	require.NoError(t, err)
	require.Equal(t, "Original purchased plan", summary.PlanName)
	require.Equal(t, "snapshot", summary.PlanNameSource)
	require.Equal(t, "presale group", summary.GroupName)
	require.Equal(t, "Current Stripe channel", summary.ProviderName)
	require.Equal(t, "USD", summary.Currency)
	require.Equal(t, "embedded", summary.PaymentMode)
	encoded, err := json.Marshal(summary)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret")
	require.NotContains(t, string(encoded), "merchant")
	require.NotContains(t, string(encoded), "999")
	require.NotContains(t, string(encoded), "quota")
}

func TestAdminOrderSummaryLegacyNamesAreExplicitlyCurrentAndMayBeMissing(t *testing.T) {
	s, _, p := newPresaleFixture(t)
	ctx := context.Background()
	o := &dbent.PaymentOrder{OrderType: payment.OrderTypeSubscription, PlanID: &p.ID, SubscriptionGroupID: &p.GroupID}
	summary, err := s.GetAdminOrderSummary(ctx, o)
	require.NoError(t, err)
	require.Equal(t, p.Name, summary.PlanName)
	require.Equal(t, "current", summary.PlanNameSource)
	require.Equal(t, "CNY", summary.Currency)
	require.Empty(t, summary.PaymentMode)
	missing := int64(999999)
	provider := "999999"
	o.PlanID, o.SubscriptionGroupID, o.ProviderInstanceID = &missing, &missing, &provider
	summary, err = s.GetAdminOrderSummary(ctx, o)
	require.NoError(t, err)
	require.Empty(t, summary.PlanName)
	require.Empty(t, summary.PlanNameSource)
	require.Empty(t, summary.GroupName)
	require.Empty(t, summary.ProviderName)
}

func TestAdminOrderSummaryBalanceDoesNotInventSubscription(t *testing.T) {
	s, _, p := newPresaleFixture(t)
	summary, err := s.GetAdminOrderSummary(context.Background(), &dbent.PaymentOrder{OrderType: payment.OrderTypeBalance, PlanID: &p.ID, PresalePlanName: "stale", ProviderSnapshot: map[string]any{"currency": "JPY"}})
	require.NoError(t, err)
	require.Empty(t, summary.PlanName)
	require.Empty(t, summary.GroupName)
	require.Equal(t, "JPY", summary.Currency)
}

func TestAdminOrderSummaryDoesNotHideDatabaseFailure(t *testing.T) {
	s, _, p := newPresaleFixture(t)
	require.NoError(t, s.entClient.Close())
	_, err := s.GetAdminOrderSummary(context.Background(), &dbent.PaymentOrder{OrderType: payment.OrderTypeSubscription, PlanID: &p.ID})
	require.ErrorContains(t, err, "load order plan name")
}
