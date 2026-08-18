package service

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	stripe "github.com/stripe/stripe-go/v85"
)

const (
	guestShopMinQty              = 1
	guestShopMaxQty              = 5
	guestShopMinPaymentAmount    = 1.0
	guestShopMaxIntentIDLength   = 128
	guestShopSubjectPrefix       = "Café Apparel Shop"
	guestShopPaymentRefPref      = "shop_"
	guestShopEnabledEnv          = "CAFE_GUEST_SHOP_ENABLED"
	guestShopStripeInstanceIDEnv = "CAFE_GUEST_SHOP_STRIPE_INSTANCE_ID"
	guestShopStripeOrderIDMeta   = "orderId"
	guestShopStripeSecretKey     = "secretKey"
	guestShopReferenceRandomSize = 16
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
	if len(items) > len(guestShopCatalog) {
		return nil, infraerrors.BadRequest("INVALID_CART", "cart contains too many distinct items")
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
	minAmount, maxAmount := guestShopPaymentBounds()
	resp := &GuestShopConfig{
		MinAmount: minAmount,
		MaxAmount: maxAmount,
		Currency:  payment.DefaultPaymentCurrency,
	}
	if !guestShopFeatureEnabled() {
		return resp, nil
	}
	selected, err := s.configService.guestShopStripeInstance(ctx)
	if err != nil {
		slog.Error("guest shop Stripe config lookup failed", "error", err)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	if selected == nil {
		return resp, nil
	}
	resp.Enabled = true
	resp.StripePublishableKey = strings.TrimSpace(selected.Config[payment.ConfigKeyPublishableKey])
	resp.Currency = paymentProviderConfigCurrency(selected.ProviderKey, selected.Config)
	return resp, nil
}

// guestShopStripeInstance returns the explicitly pinned, read-only Stripe instance
// for the standalone storefront. The original site's enabled flag is intentionally
// ignored; CAFE_GUEST_SHOP_ENABLED is the storefront's independent kill switch.
func (s *PaymentConfigService) guestShopStripeInstance(ctx context.Context) (*payment.InstanceSelection, error) {
	if s == nil || s.entClient == nil {
		return nil, nil
	}
	instanceID, ok := guestShopStripeInstanceID()
	if !ok {
		return nil, nil
	}
	instance, err := s.entClient.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.IDEQ(instanceID),
			paymentproviderinstance.ProviderKeyEQ(payment.TypeStripe),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("query pinned guest shop Stripe instance: %w", err)
	}
	cfg, err := s.decryptConfig(instance.Config)
	if err != nil {
		return nil, fmt.Errorf("decrypt pinned guest shop Stripe instance: %w", err)
	}
	if cfg == nil {
		return nil, nil
	}
	secretKey := strings.TrimSpace(cfg[guestShopStripeSecretKey])
	publishableKey := strings.TrimSpace(cfg[payment.ConfigKeyPublishableKey])
	currency, currencyErr := payment.NormalizePaymentCurrency(cfg["currency"])
	if secretKey == "" || publishableKey == "" || currencyErr != nil {
		return nil, nil
	}
	cfg[guestShopStripeSecretKey] = secretKey
	cfg[payment.ConfigKeyPublishableKey] = publishableKey
	cfg["currency"] = currency
	return &payment.InstanceSelection{
		InstanceID:     strconv.FormatInt(instance.ID, 10),
		ProviderKey:    payment.TypeStripe,
		Config:         cfg,
		SupportedTypes: instance.SupportedTypes,
		PaymentMode:    instance.PaymentMode,
	}, nil
}

func (s *PaymentService) CreateGuestShopPayment(ctx context.Context, req CreateGuestShopPaymentRequest) (*GuestShopPayment, error) {
	if s == nil || s.configService == nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	if !guestShopFeatureEnabled() {
		return nil, infraerrors.Forbidden("GUEST_SHOP_DISABLED", "guest checkout is disabled")
	}
	if err := validateGuestShopCustomer(req.Customer); err != nil {
		return nil, err
	}
	quote, err := quoteGuestShopCart(req.Items, req.Shipping)
	if err != nil {
		return nil, err
	}
	if err := validateGuestShopShippingCountry(quote.Shipping, req.Customer.Country); err != nil {
		return nil, err
	}
	minAmount, maxAmount := guestShopPaymentBounds()
	if quote.Total < minAmount || quote.Total > maxAmount {
		return nil, infraerrors.BadRequest("INVALID_AMOUNT", "amount out of range").
			WithMetadata(map[string]string{
				"min": fmt.Sprintf("%.2f", minAmount),
				"max": fmt.Sprintf("%.2f", maxAmount),
			})
	}
	sel, err := s.configService.guestShopStripeInstance(ctx)
	if err != nil {
		slog.Error("guest shop Stripe config lookup failed", "error", err)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	if sel == nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	publishableKey := strings.TrimSpace(sel.Config[payment.ConfigKeyPublishableKey])
	currency := paymentProviderConfigCurrency(sel.ProviderKey, sel.Config)
	payAmountStr := payment.FormatAmountForCurrency(quote.Total, currency)
	if _, err := payment.AmountToMinorUnit(payAmountStr, currency); err != nil {
		slog.Error("guest shop payment amount conversion failed", "currency", currency, "error", err)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "unable to start payment")
	}
	prov, err := newGuestShopProvider(sel.ProviderKey, sel.InstanceID, sel.Config)
	if err != nil {
		slog.Error("guest shop Stripe provider initialization failed", "instance_id", sel.InstanceID, "error", err)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	shopRef, err := newGuestShopPaymentReference()
	if err != nil {
		slog.Error("guest shop payment reference generation failed", "error", err)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "unable to start payment")
	}
	pr, err := prov.CreatePayment(ctx, payment.CreatePaymentRequest{
		OrderID:            shopRef,
		Amount:             payAmountStr,
		PaymentType:        payment.TypeStripe,
		Subject:            buildGuestShopSubject(quote),
		ClientIP:           req.ClientIP,
		InstanceSubMethods: selectedInstanceSupportedTypes(sel),
	})
	if err != nil {
		slog.Error("guest shop Stripe payment creation failed", "instance_id", sel.InstanceID, "error", err)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "unable to start payment")
	}
	if pr == nil || strings.TrimSpace(pr.ClientSecret) == "" {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment gateway did not return a client secret")
	}
	intentID := strings.TrimSpace(pr.TradeNo)
	if intentID == "" {
		intentID = strings.TrimSpace(pr.IntentID)
	}
	if !guestShopIntentIDRe.MatchString(intentID) || len(intentID) > guestShopMaxIntentIDLength {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment gateway did not return a valid payment intent")
	}
	return &GuestShopPayment{
		Quote:           *quote,
		Amount:          quote.Total,
		Currency:        currency,
		ClientSecret:    pr.ClientSecret,
		PublishableKey:  publishableKey,
		PaymentIntentID: intentID,
	}, nil
}

func (s *PaymentService) GetGuestShopPaymentStatus(ctx context.Context, paymentIntentID string) (*GuestShopPaymentStatus, error) {
	intentID := strings.TrimSpace(paymentIntentID)
	if len(intentID) > guestShopMaxIntentIDLength || !guestShopIntentIDRe.MatchString(intentID) {
		return nil, infraerrors.BadRequest("INVALID_PAYMENT_INTENT", "invalid payment intent")
	}
	if s == nil || s.configService == nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	sel, err := s.configService.guestShopStripeInstance(ctx)
	if err != nil {
		slog.Error("guest shop Stripe config lookup failed", "error", err)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	if sel == nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	prov, err := newGuestShopProvider(sel.ProviderKey, sel.InstanceID, sel.Config)
	if err != nil {
		slog.Warn("guest shop Stripe provider initialization failed", "instance_id", sel.InstanceID, "error", err)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "unable to query payment status")
	}
	result, err := prov.QueryOrder(ctx, intentID)
	if err != nil {
		if isGuestShopStripeNotFound(err) {
			return nil, infraerrors.NotFound("PAYMENT_NOT_FOUND", "payment not found")
		}
		slog.Warn("guest shop Stripe status query failed", "instance_id", sel.InstanceID, "error", err)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "unable to query payment status")
	}
	if result == nil || strings.TrimSpace(result.TradeNo) != intentID || !isGuestShopStripeResult(result) {
		return nil, infraerrors.NotFound("PAYMENT_NOT_FOUND", "payment not found")
	}
	currency := paymentProviderConfigCurrency(sel.ProviderKey, sel.Config)
	if resultCurrency, currencyErr := payment.NormalizePaymentCurrency(result.Metadata["currency"]); currencyErr == nil {
		currency = resultCurrency
	}
	return &GuestShopPaymentStatus{
		PaymentIntentID: intentID,
		Status:          result.Status,
		Amount:          result.Amount,
		Currency:        currency,
	}, nil
}

func guestShopFeatureEnabled() bool {
	raw := strings.TrimSpace(os.Getenv(guestShopEnabledEnv))
	if raw == "" {
		return false
	}
	enabled, err := strconv.ParseBool(raw)
	return err == nil && enabled
}

func guestShopStripeInstanceID() (int64, bool) {
	raw := strings.TrimSpace(os.Getenv(guestShopStripeInstanceIDEnv))
	instanceID, err := strconv.ParseInt(raw, 10, 64)
	return instanceID, err == nil && instanceID > 0
}

func guestShopPaymentBounds() (float64, float64) {
	maxAmount := 0.0
	for _, product := range guestShopCatalog {
		maxAmount += product.Price * guestShopMaxQty
	}
	maxShipping := 0.0
	for _, option := range guestShopShipping {
		if option.Price > maxShipping {
			maxShipping = option.Price
		}
	}
	return guestShopMinPaymentAmount, guestShopMoney(maxAmount + maxShipping)
}

func newGuestShopPaymentReference() (string, error) {
	random := make([]byte, guestShopReferenceRandomSize)
	if _, err := cryptorand.Read(random); err != nil {
		return "", err
	}
	return guestShopPaymentRefPref + hex.EncodeToString(random), nil
}

func validateGuestShopShippingCountry(shipping, country string) error {
	if strings.TrimSpace(shipping) != "domestic" {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(country)) {
	case "united states", "united states of america", "us", "usa":
		return nil
	default:
		return infraerrors.BadRequest("INVALID_SHIPPING", "US domestic shipping requires a United States address")
	}
}

func isGuestShopStripeNotFound(err error) bool {
	stripeErr := new(stripe.Error)
	return errors.As(err, &stripeErr) && stripeErr.HTTPStatusCode == http.StatusNotFound
}

func isGuestShopStripeResult(result *payment.QueryOrderResponse) bool {
	if result == nil || result.Metadata == nil {
		return false
	}
	return isGuestShopPaymentReference(result.Metadata[guestShopStripeOrderIDMeta])
}

func isGuestShopPaymentReference(reference string) bool {
	return strings.HasPrefix(strings.TrimSpace(reference), guestShopPaymentRefPref)
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
