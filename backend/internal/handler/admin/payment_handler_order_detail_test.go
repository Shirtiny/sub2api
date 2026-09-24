package admin

import (
	"encoding/json"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestAdminOrderResponseIncludesCurrencyWithoutProviderSnapshot(t *testing.T) {
	order := &dbent.PaymentOrder{ID: 9, UserEmail: "buyer@example.test", PresalePlanName: "Original name", ProviderSnapshot: map[string]any{"currency": "USD", "merchant_id": "do-not-expose"}}
	response := sanitizeAdminPaymentOrderForResponse(order)
	require.Equal(t, "USD", response.Currency)
	require.Nil(t, response.ProviderSnapshot)
	require.NotNil(t, order.ProviderSnapshot, "response sanitization must not mutate the persisted snapshot")
	data, err := json.Marshal(response)
	require.NoError(t, err)
	require.Contains(t, string(data), `"currency":"USD"`)
	require.Contains(t, string(data), `"presale_plan_name":"Original name"`)
	require.NotContains(t, string(data), "provider_snapshot")
	require.NotContains(t, string(data), "do-not-expose")
	list := sanitizeAdminPaymentOrdersForResponse([]*dbent.PaymentOrder{order})
	require.Equal(t, "USD", list[0].Currency)
	require.Empty(t, sanitizeAdminPaymentOrdersForResponse(nil))
	require.Nil(t, sanitizeAdminPaymentOrderForResponse(nil))
}
