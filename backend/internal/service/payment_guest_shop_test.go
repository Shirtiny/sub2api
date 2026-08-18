//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type guestShopTestProvider struct {
	created []payment.CreatePaymentRequest
	queryID string
}

func (p *guestShopTestProvider) Name() string        { return "guest-shop-test" }
func (p *guestShopTestProvider) ProviderKey() string { return payment.TypeStripe }
func (p *guestShopTestProvider) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeStripe}
}
func (p *guestShopTestProvider) CreatePayment(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	p.created = append(p.created, req)
	return &payment.CreatePaymentResponse{
		TradeNo:      "pi_test_guest_shop",
		ClientSecret: "pi_test_guest_shop_secret",
		Currency:     "USD",
	}, nil
}
func (p *guestShopTestProvider) QueryOrder(_ context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	p.queryID = tradeNo
	return &payment.QueryOrderResponse{
		TradeNo:  tradeNo,
		Status:   payment.ProviderStatusPaid,
		Amount:   107,
		Metadata: map[string]string{"currency": "USD"},
	}, nil
}
func (p *guestShopTestProvider) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	panic("unexpected VerifyNotification")
}
func (p *guestShopTestProvider) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	panic("unexpected Refund")
}

type guestShopTestBalancer struct {
	sel *payment.InstanceSelection
}

func (b *guestShopTestBalancer) GetInstanceConfig(context.Context, int64) (map[string]string, error) {
	if b.sel == nil {
		return map[string]string{}, nil
	}
	return b.sel.Config, nil
}

func (b *guestShopTestBalancer) SelectInstance(_ context.Context, providerKey string, paymentType payment.PaymentType, _ payment.Strategy, _ float64) (*payment.InstanceSelection, error) {
	if providerKey != payment.TypeStripe || paymentType != payment.TypeStripe {
		tpanic := "unexpected selection " + providerKey + "/" + paymentType
		panic(tpanic)
	}
	return b.sel, nil
}

func TestQuoteGuestShopCartUsesServerCatalog(t *testing.T) {
	t.Parallel()

	quote, err := quoteGuestShopCart([]GuestShopItemInput{
		{ID: "blazer", Qty: 2},
		{ID: "dress", Qty: 1},
	}, "air")
	require.NoError(t, err)
	require.Equal(t, 89.0, quote.Items[0].Price)
	require.Equal(t, 120.0, quote.Items[1].Price)
	require.Equal(t, 298.0, quote.Subtotal)
	require.Equal(t, 18.0, quote.ShippingPrice)
	require.Equal(t, 316.0, quote.Total)
	require.Equal(t, "Espresso Emerald Classic", quote.Items[0].Name)
}

func TestQuoteGuestShopCartRejectsUnknownAndInvalidQty(t *testing.T) {
	t.Parallel()

	_, err := quoteGuestShopCart([]GuestShopItemInput{{ID: "hoodie", Qty: 1}}, "air")
	require.Error(t, err)
	require.Equal(t, "INVALID_CART", infraerrors.Reason(err))

	_, err = quoteGuestShopCart([]GuestShopItemInput{{ID: "blazer", Qty: 6}}, "sea")
	require.Error(t, err)
	require.Equal(t, "INVALID_CART", infraerrors.Reason(err))

	_, err = quoteGuestShopCart([]GuestShopItemInput{{ID: "blazer", Qty: 1}}, "express")
	require.Error(t, err)
	require.Equal(t, "INVALID_SHIPPING", infraerrors.Reason(err))
}

func TestQuoteGuestShopCartDomesticShippingIsFree(t *testing.T) {
	t.Parallel()

	quote, err := quoteGuestShopCart([]GuestShopItemInput{{ID: "skirt", Qty: 1}}, "domestic")
	require.NoError(t, err)
	require.Equal(t, 255.0, quote.Total)
	require.Zero(t, quote.ShippingPrice)
}

func TestValidateOrderInputStillRejectsUnknownTypes(t *testing.T) {
	t.Parallel()

	svc := &PaymentService{}
	_, err := svc.validateOrderInput(context.Background(), CreateOrderRequest{
		Amount:    10,
		OrderType: "guest_shop",
	}, &PaymentConfig{Enabled: true, MinAmount: 1})
	require.Error(t, err)
	require.Equal(t, "INVALID_ORDER_TYPE", infraerrors.Reason(err))
}

func TestCreateGuestShopPaymentDoesNotPersistOrders(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	prov := &guestShopTestProvider{}
	orig := newGuestShopProvider
	newGuestShopProvider = func(providerKey, instanceID string, config map[string]string) (payment.Provider, error) {
		require.Equal(t, payment.TypeStripe, providerKey)
		require.Equal(t, "7", instanceID)
		require.Equal(t, "USD", config["currency"])
		return prov, nil
	}
	t.Cleanup(func() { newGuestShopProvider = orig })

	svc := &PaymentService{
		entClient: client,
		loadBalancer: &guestShopTestBalancer{sel: &payment.InstanceSelection{
			InstanceID:     "7",
			ProviderKey:    payment.TypeStripe,
			Config:         map[string]string{"currency": "USD", "publishableKey": "pk_test_shop"},
			SupportedTypes: "card,alipay",
		}},
		configService: &PaymentConfigService{
			entClient: client,
			settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
				SettingPaymentEnabled:    "true",
				SettingMinRechargeAmount: "1",
			}},
		},
	}

	resp, err := svc.CreateGuestShopPayment(ctx, CreateGuestShopPaymentRequest{
		Items:    []GuestShopItemInput{{ID: "blazer", Qty: 1}},
		Shipping: "air",
		Customer: GuestShopCustomer{
			Name:    "Ada Lovelace",
			Email:   "ada@example.com",
			Address: "541 West 1820 South",
			City:    "Provo",
			State:   "UT",
			ZIP:     "84601",
			Country: "United States",
		},
		ClientIP: "203.0.113.8",
	})
	require.NoError(t, err)
	require.Equal(t, 107.0, resp.Amount)
	require.Equal(t, "USD", resp.Currency)
	require.Equal(t, "pi_test_guest_shop_secret", resp.ClientSecret)
	require.Equal(t, "pk_test_shop", resp.PublishableKey)
	require.Equal(t, "pi_test_guest_shop", resp.PaymentIntentID)
	require.Len(t, prov.created, 1)
	require.Equal(t, "107.00", prov.created[0].Amount)
	require.Contains(t, prov.created[0].Subject, "Café Apparel Shop")
	require.Contains(t, prov.created[0].Subject, "Espresso Emerald Classic")
	require.True(t, len(prov.created[0].OrderID) > len(guestShopPaymentRefPref))
	require.Equal(t, "card,alipay", prov.created[0].InstanceSubMethods)

	count, err := client.PaymentOrder.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestGetGuestShopConfigUsesStripeInstanceCurrency(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("Stripe USD").
		SetConfig(`{"publishableKey":"pk_live_shop","secretKey":"sk_live_shop","currency":"USD"}`).
		SetSupportedTypes("card,alipay").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
		configService: &PaymentConfigService{
			entClient: client,
			settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
				SettingPaymentEnabled:    "true",
				SettingMinRechargeAmount: "1",
				SettingMaxRechargeAmount: "2000",
			}},
		},
	}

	cfg, err := svc.GetGuestShopConfig(ctx)
	require.NoError(t, err)
	require.True(t, cfg.Enabled)
	require.Equal(t, "pk_live_shop", cfg.StripePublishableKey)
	require.Equal(t, "USD", cfg.Currency)
	require.Equal(t, 1.0, cfg.MinAmount)
	require.Equal(t, 2000.0, cfg.MaxAmount)
}

func TestGetGuestShopPaymentStatusQueriesStripeIntent(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	prov := &guestShopTestProvider{}
	orig := newGuestShopProvider
	newGuestShopProvider = func(string, string, map[string]string) (payment.Provider, error) {
		return prov, nil
	}
	t.Cleanup(func() { newGuestShopProvider = orig })

	svc := &PaymentService{
		entClient: client,
		loadBalancer: &guestShopTestBalancer{sel: &payment.InstanceSelection{
			InstanceID:  "7",
			ProviderKey: payment.TypeStripe,
			Config:      map[string]string{"currency": "USD", "publishableKey": "pk_test_shop"},
		}},
		configService: &PaymentConfigService{
			entClient: client,
			settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{
				SettingPaymentEnabled: "true",
			}},
		},
	}

	status, err := svc.GetGuestShopPaymentStatus(ctx, "pi_test_guest_shop")
	require.NoError(t, err)
	require.Equal(t, "pi_test_guest_shop", status.PaymentIntentID)
	require.Equal(t, payment.ProviderStatusPaid, status.Status)
	require.Equal(t, 107.0, status.Amount)
	require.Equal(t, "USD", status.Currency)
	require.Equal(t, "pi_test_guest_shop", prov.queryID)

	_, err = svc.GetGuestShopPaymentStatus(ctx, "not-an-intent")
	require.Error(t, err)
	require.Equal(t, "INVALID_PAYMENT_INTENT", infraerrors.Reason(err))
}
