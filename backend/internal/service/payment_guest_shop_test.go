//go:build unit

package service

import (
	"context"
	"strconv"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type guestShopTestProvider struct {
	created     []payment.CreatePaymentRequest
	queryID     string
	currency    string
	queryResult *payment.QueryOrderResponse
	queryErr    error
}

func (p *guestShopTestProvider) Name() string        { return "guest-shop-test" }
func (p *guestShopTestProvider) ProviderKey() string { return payment.TypeStripe }
func (p *guestShopTestProvider) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeStripe}
}
func (p *guestShopTestProvider) CreatePayment(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	p.created = append(p.created, req)
	currency := p.currency
	if currency == "" {
		currency = "USD"
	}
	return &payment.CreatePaymentResponse{
		TradeNo:      "pi_test_guest_shop",
		ClientSecret: "pi_test_guest_shop_secret",
		Currency:     currency,
	}, nil
}
func (p *guestShopTestProvider) QueryOrder(_ context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	p.queryID = tradeNo
	if p.queryErr != nil {
		return nil, p.queryErr
	}
	if p.queryResult != nil {
		return p.queryResult, nil
	}
	return &payment.QueryOrderResponse{
		TradeNo:  tradeNo,
		Status:   payment.ProviderStatusPaid,
		Amount:   107,
		Metadata: map[string]string{"currency": "USD", guestShopStripeOrderIDMeta: "shop_test"},
	}, nil
}
func (p *guestShopTestProvider) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	panic("unexpected VerifyNotification")
}
func (p *guestShopTestProvider) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	panic("unexpected Refund")
}

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

func newGuestShopTestService(client *dbent.Client, enabled bool, instanceID int64) *PaymentService {
	return newGuestShopTestServiceWithSettings(client, map[string]string{
		SettingGuestShopEnabled:  strconv.FormatBool(enabled),
		SettingGuestShopStripeID: strconv.FormatInt(instanceID, 10),
	})
}

func newGuestShopTestServiceWithSettings(client *dbent.Client, values map[string]string) *PaymentService {
	return &PaymentService{
		entClient: client,
		configService: &PaymentConfigService{
			entClient:   client,
			settingRepo: &paymentConfigSettingRepoStub{values: values},
		},
	}
}

func validGuestShopCustomer(country string) GuestShopCustomer {
	return GuestShopCustomer{
		Name:    "Ada Lovelace",
		Email:   "ada@example.com",
		Address: "541 West 1820 South",
		City:    "Provo",
		State:   "UT",
		ZIP:     "84601",
		Country: country,
	}
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

func TestQuoteGuestShopCartRejectsInvalidInput(t *testing.T) {
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

	_, err = quoteGuestShopCart([]GuestShopItemInput{
		{ID: "blazer", Qty: 1},
		{ID: "skirt", Qty: 1},
		{ID: "dress", Qty: 1},
		{ID: "blazer", Qty: 1},
	}, "air")
	require.Error(t, err)
	require.Equal(t, "INVALID_CART", infraerrors.Reason(err))
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

func TestCreateGuestShopPaymentUsesPinnedStripeAndDoesNotPersistOrders(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_ = createGuestShopStripeInstance(t, client, "Stripe secondary", "pk_secondary", "sk_secondary", "USD", 1, true)
	primaryID := createGuestShopStripeInstance(t, client, "Stripe storefront", "pk_storefront", "sk_storefront", "USD", 20, false)
	prov := &guestShopTestProvider{}
	orig := newGuestShopProvider
	newGuestShopProvider = func(providerKey, instanceID string, config map[string]string) (payment.Provider, error) {
		require.Equal(t, payment.TypeStripe, providerKey)
		require.Equal(t, strconv.FormatInt(primaryID, 10), instanceID)
		require.Equal(t, "pk_storefront", config[payment.ConfigKeyPublishableKey])
		require.Equal(t, "sk_storefront", config[guestShopStripeSecretKey])
		require.Equal(t, "USD", config["currency"])
		return prov, nil
	}
	t.Cleanup(func() { newGuestShopProvider = orig })

	svc := newGuestShopTestService(client, true, primaryID)
	resp, err := svc.CreateGuestShopPayment(ctx, CreateGuestShopPaymentRequest{
		Items:    []GuestShopItemInput{{ID: "blazer", Qty: 1}},
		Shipping: "air",
		Customer: validGuestShopCustomer("United States"),
		ClientIP: "203.0.113.8",
	})
	require.NoError(t, err)
	require.Equal(t, 107.0, resp.Amount)
	require.Equal(t, "USD", resp.Currency)
	require.Equal(t, "pi_test_guest_shop_secret", resp.ClientSecret)
	require.Equal(t, "pk_storefront", resp.PublishableKey)
	require.Equal(t, "pi_test_guest_shop", resp.PaymentIntentID)
	require.Len(t, prov.created, 1)
	require.Equal(t, "107.00", prov.created[0].Amount)
	require.Contains(t, prov.created[0].Subject, "Café Apparel Shop")
	require.Contains(t, prov.created[0].Subject, "Espresso Emerald Classic")
	require.Regexp(t, `^shop_[0-9a-f]{32}$`, prov.created[0].OrderID)
	require.Equal(t, "card,alipay", prov.created[0].InstanceSubMethods)

	count, err := client.PaymentOrder.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
	userCount, err := client.User.Query().Count(ctx)
	require.NoError(t, err)
	require.Zero(t, userCount)
}

func TestGetGuestShopConfigIsIndependentFromOriginalPaymentSettings(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_ = createGuestShopStripeInstance(t, client, "Stripe secondary", "pk_usd", "sk_usd", "USD", 1, true)
	pinnedID := createGuestShopStripeInstance(t, client, "Stripe storefront", "pk_eur", "sk_eur", "EUR", 20, false)

	svc := newGuestShopTestServiceWithSettings(client, map[string]string{
		SettingPaymentEnabled:    "false",
		SettingMinRechargeAmount: "999",
		SettingMaxRechargeAmount: "1000",
		SettingGuestShopEnabled:  "true",
		SettingGuestShopStripeID: strconv.FormatInt(pinnedID, 10),
	})
	cfg, err := svc.GetGuestShopConfig(ctx)
	require.NoError(t, err)
	require.True(t, cfg.Enabled)
	require.Equal(t, "pk_eur", cfg.StripePublishableKey)
	require.Equal(t, "EUR", cfg.Currency)
	require.Equal(t, 1.0, cfg.MinAmount)
	require.Equal(t, 2338.0, cfg.MaxAmount)
}

func TestGetGuestShopConfigDoesNotFallbackFromIncompletePinnedInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	incompleteID := createGuestShopStripeInstance(t, client, "Stripe incomplete", "", "sk_incomplete", "USD", 1, false)
	_ = createGuestShopStripeInstance(t, client, "Stripe storefront", "pk_storefront", "sk_storefront", "USD", 2, true)

	cfg, err := newGuestShopTestService(client, true, incompleteID).GetGuestShopConfig(ctx)
	require.NoError(t, err)
	require.False(t, cfg.Enabled)
	require.Empty(t, cfg.StripePublishableKey)
}

func TestGetGuestShopPaymentStatusUsesPinnedDisabledStripeInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_ = createGuestShopStripeInstance(t, client, "Stripe first", "pk_first", "sk_first", "USD", 1, true)
	pinnedID := createGuestShopStripeInstance(t, client, "Stripe pinned", "pk_pinned", "sk_pinned", "USD", 2, false)
	pinned := &guestShopTestProvider{}
	orig := newGuestShopProvider
	newGuestShopProvider = func(_ string, instanceID string, _ map[string]string) (payment.Provider, error) {
		require.Equal(t, strconv.FormatInt(pinnedID, 10), instanceID)
		return pinned, nil
	}
	t.Cleanup(func() { newGuestShopProvider = orig })

	status, err := newGuestShopTestService(client, false, pinnedID).GetGuestShopPaymentStatus(ctx, "pi_test_guest_shop")
	require.NoError(t, err)
	require.Equal(t, "pi_test_guest_shop", status.PaymentIntentID)
	require.Equal(t, payment.ProviderStatusPaid, status.Status)
	require.Equal(t, 107.0, status.Amount)
	require.Equal(t, "USD", status.Currency)
	require.Equal(t, "pi_test_guest_shop", pinned.queryID)
}

func TestGetGuestShopPaymentStatusRejectsNonGuestIntent(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	pinnedID := createGuestShopStripeInstance(t, client, "Stripe storefront", "pk_storefront", "sk_storefront", "USD", 1, false)
	prov := &guestShopTestProvider{queryResult: &payment.QueryOrderResponse{
		TradeNo:  "pi_normal_order",
		Status:   payment.ProviderStatusPaid,
		Amount:   107,
		Metadata: map[string]string{"currency": "USD", guestShopStripeOrderIDMeta: "sub2_123"},
	}}
	orig := newGuestShopProvider
	newGuestShopProvider = func(string, string, map[string]string) (payment.Provider, error) {
		return prov, nil
	}
	t.Cleanup(func() { newGuestShopProvider = orig })

	_, err := newGuestShopTestService(client, true, pinnedID).GetGuestShopPaymentStatus(ctx, "pi_normal_order")
	require.Error(t, err)
	require.Equal(t, "PAYMENT_NOT_FOUND", infraerrors.Reason(err))
}

func TestGetGuestShopConfigFailsClosedWithoutValidPinnedInstance(t *testing.T) {
	client := newPaymentConfigServiceTestClient(t)
	_ = createGuestShopStripeInstance(t, client, "Stripe available", "pk_available", "sk_available", "USD", 1, true)
	wrongProvider, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("Alipay instance").
		SetConfig(`{}`).
		SetSupportedTypes(payment.TypeAlipay).
		SetEnabled(true).
		Save(context.Background())
	require.NoError(t, err)
	tests := map[string]string{
		"missing":        "",
		"invalid":        "not-an-id",
		"zero":           "0",
		"negative":       "-1",
		"unknown":        "999999",
		"wrong-provider": strconv.FormatInt(wrongProvider.ID, 10),
	}
	for name, instanceID := range tests {
		t.Run(name, func(t *testing.T) {
			svc := newGuestShopTestServiceWithSettings(client, map[string]string{
				SettingGuestShopEnabled:  "true",
				SettingGuestShopStripeID: instanceID,
			})
			cfg, err := svc.GetGuestShopConfig(context.Background())
			require.NoError(t, err)
			require.False(t, cfg.Enabled)
			require.Empty(t, cfg.StripePublishableKey)
		})
	}
}

func TestCreateGuestShopPaymentRejectsDomesticShippingOutsideUS(t *testing.T) {
	svc := newGuestShopTestService(nil, true, 1)
	_, err := svc.CreateGuestShopPayment(context.Background(), CreateGuestShopPaymentRequest{
		Items:    []GuestShopItemInput{{ID: "blazer", Qty: 1}},
		Shipping: "domestic",
		Customer: validGuestShopCustomer("China"),
	})
	require.Error(t, err)
	require.Equal(t, "INVALID_SHIPPING", infraerrors.Reason(err))
}

func TestGuestShopFeatureFlagOnlyDisablesGuestCheckout(t *testing.T) {
	svc := newGuestShopTestService(nil, false, 0)

	cfg, err := svc.GetGuestShopConfig(context.Background())
	require.NoError(t, err)
	require.False(t, cfg.Enabled)
	require.Equal(t, 1.0, cfg.MinAmount)
	require.Equal(t, 2338.0, cfg.MaxAmount)

	_, err = svc.CreateGuestShopPayment(context.Background(), CreateGuestShopPaymentRequest{
		Items:    []GuestShopItemInput{{ID: "blazer", Qty: 1}},
		Shipping: "air",
		Customer: validGuestShopCustomer("United States"),
	})
	require.Error(t, err)
	require.Equal(t, "GUEST_SHOP_DISABLED", infraerrors.Reason(err))
}

func TestGetGuestShopPaymentStatusRejectsInvalidIntent(t *testing.T) {
	t.Parallel()

	svc := &PaymentService{}
	_, err := svc.GetGuestShopPaymentStatus(context.Background(), "not-an-intent")
	require.Error(t, err)
	require.Equal(t, "INVALID_PAYMENT_INTENT", infraerrors.Reason(err))

	_, err = svc.GetGuestShopPaymentStatus(context.Background(), "pi_"+string(make([]byte, guestShopMaxIntentIDLength)))
	require.Error(t, err)
	require.Equal(t, "INVALID_PAYMENT_INTENT", infraerrors.Reason(err))
}
