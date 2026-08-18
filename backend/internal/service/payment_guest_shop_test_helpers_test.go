package service

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func createGuestShopStripeInstance(
	t *testing.T,
	client *dbent.Client,
	name, publishableKey, secretKey, currency string,
	sortOrder int,
	enabled bool,
) int64 {
	t.Helper()
	instance, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName(name).
		SetConfig(`{"publishableKey":"` + publishableKey + `","secretKey":"` + secretKey + `","currency":"` + currency + `"}`).
		SetSupportedTypes("card,alipay").
		SetSortOrder(sortOrder).
		SetEnabled(enabled).
		Save(context.Background())
	require.NoError(t, err)
	return instance.ID
}
