package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	guestShopMinQty         = 1
	guestShopMaxQty         = 5
	guestShopSubjectPrefix  = "Café Apparel Shop"
	guestShopPaymentRefPref = "shop_"
)

var (
	guestShopEmailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	guestShopIntentIDRe   = regexp.MustCompile(`^pi_[A-Za-z0-9_]+$`)
	newGuestShopProvider  = provider.CreateProvider
)

type guestShopProduct struct {
	ID    string
	Name  string
	Price float64
}

type guestShopShippingOption struct {
	Label string
	Price float64
}

var guestShopCatalog = map[string]guestShopProduct{
	"blazer": {ID: "blazer", Name: "Espresso Emerald Classic", Price: 89},
	"skirt":  {ID: "skirt", Name: "Full Collection Style Pack", Price: 255},
	"dress":  {ID: "dress", Name: "Matcha Lace Playful Style", Price: 120},
}

var guestShopShipping = map[string]guestShopShippingOption{
	"air":      {Label: "Air freight · 7–14 business days", Price: 18},
	"sea":      {Label: "Sea freight · 25–45 business days", Price: 8},
	"domestic": {Label: "US domestic shipping · 3–7 business days", Price: 0},
}

type GuestShopItemInput struct {
	ID  string `json:"id"`
	Qty int    `json:"qty"`
}

type GuestShopCustomer struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Address string `json:"address"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZIP     string `json:"zip"`
	Country string `json:"country"`
}

type CreateGuestShopPaymentRequest struct {
	Items    []GuestShopItemInput
	Shipping string
	Customer GuestShopCustomer
	ClientIP string
}

type GuestShopQuotedItem struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Qty   int     `json:"qty"`
}

type GuestShopQuote struct {
	Items         []GuestShopQuotedItem `json:"items"`
	Shipping      string                `json:"shipping"`
	ShippingLabel string                `json:"shipping_label"`
	Subtotal      float64               `json:"subtotal"`
	ShippingPrice float64               `json:"shipping_price"`
	Total         float64               `json:"total"`
}

type GuestShopConfig struct {
	Enabled              bool    `json:"enabled"`
	StripePublishableKey string  `json:"stripe_publishable_key"`
	MinAmount            float64 `json:"min_amount"`
	MaxAmount            float64 `json:"max_amount"`
	Currency             string  `json:"currency"`
}

type GuestShopPayment struct {
	Quote           GuestShopQuote `json:"quote"`
	Amount          float64        `json:"amount"`
	Currency        string         `json:"currency"`
	ClientSecret    string         `json:"client_secret"`
	PublishableKey  string         `json:"stripe_publishable_key"`
	PaymentIntentID string         `json:"payment_intent_id"`
}

type GuestShopPaymentStatus struct {
	PaymentIntentID string  `json:"payment_intent_id"`
	Status          string  `json:"status"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency"`
}

func quoteGuestShopCart(items []GuestShopItemInput, shipping string) (*GuestShopQuote, error) {
	if len(items) == 0 {
		return nil, infraerrors.BadRequest("INVALID_CART", "cart must contain at least one item")
	}
	ship, ok := guestShopShipping[strings.TrimSpace(shipping)]
	if !ok {
		return nil, infraerrors.BadRequest("INVALID_SHIPPING", "unsupported shipping method")
	}
	seen := make(map[string]struct{}, len(items))
	quoted := make([]GuestShopQuotedItem, 0, len(items))
	subtotal := 0.0
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			return nil, infraerrors.BadRequest("INVALID_CART", "item id is required")
		}
		if _, dup := seen[id]; dup {
			return nil, infraerrors.BadRequest("INVALID_CART", "duplicate item in cart")
		}
		product, exists := guestShopCatalog[id]
		if !exists {
			return nil, infraerrors.BadRequest("INVALID_CART", "unknown item").
				WithMetadata(map[string]string{"item_id": id})
		}
		if item.Qty < guestShopMinQty || item.Qty > guestShopMaxQty {
			return nil, infraerrors.BadRequest("INVALID_CART", "item quantity must be between 1 and 5").
				WithMetadata(map[string]string{"item_id": id})
		}
		seen[id] = struct{}{}
		line := guestShopMoney(product.Price * float64(item.Qty))
		subtotal += line
		quoted = append(quoted, GuestShopQuotedItem{
			ID:    product.ID,
			Name:  product.Name,
			Price: product.Price,
			Qty:   item.Qty,
		})
	}
	subtotal = guestShopMoney(subtotal)
	shipPrice := guestShopMoney(ship.Price)
	return &GuestShopQuote{
		Items:         quoted,
		Shipping:      strings.TrimSpace(shipping),
		ShippingLabel: ship.Label,
		Subtotal:      subtotal,
		ShippingPrice: shipPrice,
		Total:         guestShopMoney(subtotal + shipPrice),
	}, nil
}

func guestShopMoney(v float64) float64 {
	return math.Round(v*100) / 100
}

func (s *PaymentService) GetGuestShopConfig(ctx context.Context) (*GuestShopConfig, error) {
	if s == nil || s.configService == nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, err
	}
	publishableKey := strings.TrimSpace(cfg.StripePublishableKey)
	currency := s.configService.firstEnabledStripeCurrency(ctx)
	enabled := cfg.Enabled && publishableKey != ""
	return &GuestShopConfig{
		Enabled:              enabled,
		StripePublishableKey: publishableKey,
		MinAmount:            cfg.MinAmount,
		MaxAmount:            cfg.MaxAmount,
		Currency:             currency,
	}, nil
}

func (s *PaymentConfigService) firstEnabledStripeCurrency(ctx context.Context) string {
	if s == nil || s.entClient == nil {
		return payment.DefaultPaymentCurrency
	}
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.EnabledEQ(true),
			paymentproviderinstance.ProviderKeyEQ(payment.TypeStripe),
		).Limit(1).All(ctx)
	if err != nil || len(instances) == 0 {
		return payment.DefaultPaymentCurrency
	}
	cfg, err := s.decryptConfig(instances[0].Config)
	if err != nil || cfg == nil {
		return payment.DefaultPaymentCurrency
	}
	return paymentProviderConfigCurrency(payment.TypeStripe, cfg)
}

func (s *PaymentService) CreateGuestShopPayment(ctx context.Context, req CreateGuestShopPaymentRequest) (*GuestShopPayment, error) {
	if s == nil || s.configService == nil || s.loadBalancer == nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	if err := validateGuestShopCustomer(req.Customer); err != nil {
		return nil, err
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, infraerrors.Forbidden("PAYMENT_DISABLED", "payment system is disabled")
	}
	quote, err := quoteGuestShopCart(req.Items, req.Shipping)
	if err != nil {
		return nil, err
	}
	if (cfg.MinAmount > 0 && quote.Total < cfg.MinAmount) || (cfg.MaxAmount > 0 && quote.Total > cfg.MaxAmount) {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "amount out of range").
			WithMetadata(map[string]string{
				"min": fmt.Sprintf("%.2f", cfg.MinAmount),
				"max": fmt.Sprintf("%.2f", cfg.MaxAmount),
			})
	}
	sel, err := s.selectGuestShopStripeInstance(ctx, cfg, quote.Total)
	if err != nil {
		return nil, err
	}
	currency := paymentProviderConfigCurrency(sel.ProviderKey, sel.Config)
	payAmountStr := payment.FormatAmountForCurrency(quote.Total, currency)
	if err := validateSelectedCreateOrderAmountCurrency(payAmountStr, sel); err != nil {
		return nil, err
	}
	if quote.Total < minCreateOrderPayAmount {
		minAmount := payment.FormatAmountForCurrency(minCreateOrderPayAmount, currency)
		return nil, infraerrors.BadRequest("PAYMENT_AMOUNT_BELOW_MINIMUM", "payment amount must be at least "+minAmount).
			WithMetadata(map[string]string{"min": minAmount, "currency": currency})
	}
	prov, err := newGuestShopProvider(sel.ProviderKey, sel.InstanceID, sel.Config)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_PROVIDER_MISCONFIGURED", "provider_misconfigured").
			WithMetadata(map[string]string{"provider": sel.ProviderKey, "instance_id": sel.InstanceID})
	}
	shopRef := guestShopPaymentRefPref + generateRandomString(16)
	pr, err := prov.CreatePayment(ctx, payment.CreatePaymentRequest{
		OrderID:            shopRef,
		Amount:             payAmountStr,
		PaymentType:        payment.TypeStripe,
		Subject:            buildGuestShopSubject(quote),
		ClientIP:           req.ClientIP,
		InstanceSubMethods: selectedInstanceSupportedTypes(sel),
	})
	if err != nil {
		if appErr := new(infraerrors.ApplicationError); errors.As(err, &appErr) {
			return nil, appErr
		}
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", fmt.Sprintf("payment gateway error: %s", err.Error()))
	}
	if pr == nil || strings.TrimSpace(pr.ClientSecret) == "" {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment gateway did not return a client secret")
	}
	intentID := strings.TrimSpace(pr.TradeNo)
	if intentID == "" {
		intentID = strings.TrimSpace(pr.IntentID)
	}
	publishableKey := strings.TrimSpace(cfg.StripePublishableKey)
	if publishableKey == "" {
		publishableKey = strings.TrimSpace(sel.Config[payment.ConfigKeyPublishableKey])
	}
	if publishableKey == "" {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "stripe publishable key is not configured")
	}
	respCurrency := strings.TrimSpace(pr.Currency)
	if respCurrency == "" {
		respCurrency = currency
	}
	return &GuestShopPayment{
		Quote:           *quote,
		Amount:          quote.Total,
		Currency:        respCurrency,
		ClientSecret:    pr.ClientSecret,
		PublishableKey:  publishableKey,
		PaymentIntentID: intentID,
	}, nil
}

func (s *PaymentService) GetGuestShopPaymentStatus(ctx context.Context, paymentIntentID string) (*GuestShopPaymentStatus, error) {
	intentID := strings.TrimSpace(paymentIntentID)
	if !guestShopIntentIDRe.MatchString(intentID) {
		return nil, infraerrors.BadRequest("INVALID_PAYMENT_INTENT", "invalid payment intent")
	}
	if s == nil || s.configService == nil || s.loadBalancer == nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	cfg, err := s.configService.GetPaymentConfig(ctx)
	if err != nil {
		return nil, err
	}
	sel, err := s.selectGuestShopStripeInstance(ctx, cfg, minCreateOrderPayAmount)
	if err != nil {
		return nil, err
	}
	prov, err := newGuestShopProvider(sel.ProviderKey, sel.InstanceID, sel.Config)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_PROVIDER_MISCONFIGURED", "provider_misconfigured")
	}
	result, err := prov.QueryOrder(ctx, intentID)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "unable to query payment status")
	}
	if result == nil {
		return nil, infraerrors.NotFound("PAYMENT_NOT_FOUND", "payment not found")
	}
	currency := strings.TrimSpace(result.Metadata["currency"])
	if currency == "" {
		currency = paymentProviderConfigCurrency(sel.ProviderKey, sel.Config)
	}
	return &GuestShopPaymentStatus{
		PaymentIntentID: result.TradeNo,
		Status:          result.Status,
		Amount:          result.Amount,
		Currency:        currency,
	}, nil
}

func (s *PaymentService) selectGuestShopStripeInstance(ctx context.Context, cfg *PaymentConfig, payAmount float64) (*payment.InstanceSelection, error) {
	if s == nil || s.loadBalancer == nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "method_not_configured")
	}
	strategy := payment.DefaultLoadBalanceStrategy
	if cfg != nil && strings.TrimSpace(cfg.LoadBalanceStrategy) != "" {
		strategy = cfg.LoadBalanceStrategy
	}
	sel, err := s.loadBalancer.SelectInstance(ctx, payment.TypeStripe, payment.TypeStripe, payment.Strategy(strategy), payAmount)
	if err != nil || sel == nil || sel.ProviderKey != payment.TypeStripe {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "method_not_configured").
			WithMetadata(map[string]string{"payment_type": payment.TypeStripe})
	}
	return sel, nil
}

func buildGuestShopSubject(quote *GuestShopQuote) string {
	if quote == nil || len(quote.Items) == 0 {
		return guestShopSubjectPrefix
	}
	parts := make([]string, 0, len(quote.Items)+1)
	for _, item := range quote.Items {
		parts = append(parts, fmt.Sprintf("%s x%d", item.Name, item.Qty))
	}
	if quote.ShippingLabel != "" {
		parts = append(parts, quote.ShippingLabel)
	}
	return guestShopSubjectPrefix + " — " + strings.Join(parts, ", ")
}

func validateGuestShopCustomer(customer GuestShopCustomer) error {
	name := strings.TrimSpace(customer.Name)
	email := strings.TrimSpace(customer.Email)
	address := strings.TrimSpace(customer.Address)
	city := strings.TrimSpace(customer.City)
	state := strings.TrimSpace(customer.State)
	zip := strings.TrimSpace(customer.ZIP)
	country := strings.TrimSpace(customer.Country)
	if name == "" || email == "" || address == "" || city == "" || state == "" || zip == "" || country == "" {
		return infraerrors.BadRequest("INVALID_CUSTOMER", "complete shipping details are required")
	}
	if len(name) > 80 || len(email) > 120 || len(address) > 200 || len(city) > 80 || len(state) > 80 || len(zip) > 20 || len(country) > 80 {
		return infraerrors.BadRequest("INVALID_CUSTOMER", "shipping details are too long")
	}
	if !guestShopEmailPattern.MatchString(email) {
		return infraerrors.BadRequest("INVALID_CUSTOMER", "please enter a valid email address")
	}
	return nil
}
