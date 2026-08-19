package service

import (
	"context"
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

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
	guestShopPaymentRefV1Pref    = "shop_v1_"
	guestShopStripeOrderIDMeta   = "orderId"
	guestShopStripeSecretKey     = "secretKey"
	guestShopReferenceRandomSize = 16
	guestShopRouteTokenVersion   = 1
	guestShopRouteTokenMaxLength = 1024

	// GuestShopRouteTokenTTL is also the grace period during which a Stripe
	// instance that was just unselected, or whose guest checkout was disabled,
	// cannot have its secret key changed or be deleted. This keeps redirected
	// payments bound to their creation-time instance without a local order.
	GuestShopRouteTokenTTL = 24 * time.Hour
)

var (
	guestShopEmailPattern       = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	guestShopIntentIDRe         = regexp.MustCompile(`^pi_[A-Za-z0-9_]+$`)
	guestShopPaymentRefV1Re     = regexp.MustCompile(`^shop_v1_([1-9][0-9]*)_([0-9a-f]{32})$`)
	guestShopPaymentRefLegacyRe = regexp.MustCompile(`^shop_[0-9a-f]{32}$`)
	newGuestShopProvider        = provider.CreateProvider
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
	"blazer":   {ID: "blazer", Name: "Espresso Emerald Classic", Price: 89},
	"skirt":    {ID: "skirt", Name: "Full Collection Style Pack", Price: 255},
	"dress":    {ID: "dress", Name: "Matcha Lace Playful Style", Price: 120},
	"rose":     {ID: "rose", Name: "Rose Latte Service Dress", Price: 128},
	"midnight": {ID: "midnight", Name: "Midnight Mocha Tailored Uniform", Price: 148},
	"apron":    {ID: "apron", Name: "Vanilla Cream Apron Set", Price: 72},
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
	CheckoutToken   string         `json:"checkout_token"`
}

type GuestShopPaymentStatus struct {
	PaymentIntentID string  `json:"payment_intent_id"`
	Status          string  `json:"status"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency"`
}

type guestShopSettings struct {
	Enabled          bool
	StripeInstanceID int64
}

type guestShopSettingsUpdate struct {
	Previous guestShopSettings
	Next     guestShopSettings
}

type guestShopInstanceProtection struct {
	InstanceID int64 `json:"instance_id"`
	Until      int64 `json:"until"`
}

type guestShopRouteClaims struct {
	Version          int    `json:"v"`
	InstanceID       int64  `json:"iid"`
	PaymentIntentID  string `json:"pi"`
	PaymentReference string `json:"ref"`
	ExpiresAt        int64  `json:"exp"`
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
	settings, err := s.configService.guestShopSettings(ctx)
	if err != nil {
		slog.Error("guest shop settings lookup failed", "error", err)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	if !settings.Enabled {
		return resp, nil
	}
	selected, err := s.configService.guestShopStripeInstance(ctx, settings.StripeInstanceID)
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

func (s *PaymentConfigService) guestShopSettings(ctx context.Context) (guestShopSettings, error) {
	if s == nil || s.settingRepo == nil {
		return guestShopSettings{}, nil
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{SettingGuestShopEnabled, SettingGuestShopStripeID})
	if err != nil {
		return guestShopSettings{}, fmt.Errorf("get guest shop payment settings: %w", err)
	}
	return guestShopSettings{
		Enabled:          values[SettingGuestShopEnabled] == "true",
		StripeInstanceID: pcParsePositiveInt64(values[SettingGuestShopStripeID]),
	}, nil
}

// guestShopStripeInstance returns the explicitly pinned, read-only Stripe
// instance for the standalone storefront. The original site's enabled flag is
// intentionally ignored; the database-backed guest setting is the independent
// storefront kill switch.
func (s *PaymentConfigService) guestShopStripeInstance(ctx context.Context, instanceID int64) (*payment.InstanceSelection, error) {
	if s == nil || s.entClient == nil {
		return nil, nil
	}
	if instanceID <= 0 {
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
	secretKey, publishableKey, currency, complete := guestShopStripeConfigValues(cfg)
	if !complete {
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

func guestShopStripeConfigValues(config map[string]string) (secretKey, publishableKey, currency string, complete bool) {
	secretKey = strings.TrimSpace(config[guestShopStripeSecretKey])
	publishableKey = strings.TrimSpace(config[payment.ConfigKeyPublishableKey])
	currency, err := payment.NormalizePaymentCurrency(config["currency"])
	return secretKey, publishableKey, currency, secretKey != "" && publishableKey != "" && err == nil
}

func (s *PaymentConfigService) resolveGuestShopSettingsUpdate(ctx context.Context, enabled *bool, instanceID *int64) (guestShopSettingsUpdate, error) {
	previous, err := s.guestShopSettings(ctx)
	if err != nil {
		return guestShopSettingsUpdate{}, err
	}
	next := previous
	if enabled != nil {
		next.Enabled = *enabled
	}
	if instanceID != nil {
		if *instanceID < 0 {
			return guestShopSettingsUpdate{}, infraerrors.BadRequest("INVALID_GUEST_SHOP_STRIPE_INSTANCE", "guest shop Stripe instance ID cannot be negative")
		}
		next.StripeInstanceID = *instanceID
	}
	if next.Enabled && next.StripeInstanceID <= 0 {
		return guestShopSettingsUpdate{}, infraerrors.BadRequest("INVALID_GUEST_SHOP_STRIPE_INSTANCE", "select a Stripe instance before enabling guest checkout")
	}
	// The kill switch must remain usable even if the selected instance was
	// deleted or became unreadable. Revalidate when enabling or changing IDs.
	if !next.Enabled && (instanceID == nil || next.StripeInstanceID == previous.StripeInstanceID) {
		return guestShopSettingsUpdate{Previous: previous, Next: next}, nil
	}
	if next.StripeInstanceID <= 0 {
		return guestShopSettingsUpdate{Previous: previous, Next: next}, nil
	}
	selected, err := s.guestShopStripeInstance(ctx, next.StripeInstanceID)
	if err != nil {
		return guestShopSettingsUpdate{}, err
	}
	if selected == nil {
		return guestShopSettingsUpdate{}, infraerrors.BadRequest("INVALID_GUEST_SHOP_STRIPE_INSTANCE", "selected Stripe instance is missing or incomplete")
	}
	return guestShopSettingsUpdate{Previous: previous, Next: next}, nil
}

func (s *PaymentConfigService) buildGuestShopSettingsUpdates(ctx context.Context, enabled *bool, instanceID *int64, now time.Time) (map[string]string, error) {
	resolved, err := s.resolveGuestShopSettingsUpdate(ctx, enabled, instanceID)
	if err != nil {
		return nil, err
	}
	updates := make(map[string]string, 3)
	if enabled != nil {
		updates[SettingGuestShopEnabled] = formatBoolOrEmpty(enabled)
	}
	if instanceID != nil {
		updates[SettingGuestShopStripeID] = formatNonNegativeInt64(instanceID)
	}
	shouldProtectPrevious := resolved.Previous.Enabled && resolved.Previous.StripeInstanceID > 0 &&
		(!resolved.Next.Enabled || resolved.Previous.StripeInstanceID != resolved.Next.StripeInstanceID)
	if !shouldProtectPrevious {
		return updates, nil
	}

	protections, err := s.loadGuestShopInstanceProtections(ctx)
	if err != nil {
		return nil, err
	}
	protections = addGuestShopInstanceProtection(
		protections,
		resolved.Previous.StripeInstanceID,
		now.Add(GuestShopRouteTokenTTL).Unix(),
		now,
	)
	encoded, err := json.Marshal(protections)
	if err != nil {
		return nil, fmt.Errorf("marshal guest shop Stripe protection: %w", err)
	}
	updates[SettingGuestShopProtection] = string(encoded)
	return updates, nil
}

func (s *PaymentConfigService) loadGuestShopInstanceProtections(ctx context.Context) ([]guestShopInstanceProtection, error) {
	if s == nil || s.settingRepo == nil {
		return nil, nil
	}
	value, err := s.settingRepo.GetValue(ctx, SettingGuestShopProtection)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get guest shop Stripe protection: %w", err)
	}
	return parseGuestShopInstanceProtections(value)
}

func parseGuestShopInstanceProtections(raw string) ([]guestShopInstanceProtection, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var protections []guestShopInstanceProtection
	if err := json.Unmarshal([]byte(raw), &protections); err != nil {
		return nil, fmt.Errorf("parse guest shop Stripe protection: %w", err)
	}
	for _, protection := range protections {
		if protection.InstanceID <= 0 || protection.Until <= 0 {
			return nil, fmt.Errorf("parse guest shop Stripe protection: invalid entry")
		}
	}
	return protections, nil
}

func addGuestShopInstanceProtection(protections []guestShopInstanceProtection, instanceID, until int64, now time.Time) []guestShopInstanceProtection {
	byID := make(map[int64]int64, len(protections)+1)
	for _, protection := range protections {
		if protection.InstanceID > 0 && protection.Until > now.Unix() {
			if protection.Until > byID[protection.InstanceID] {
				byID[protection.InstanceID] = protection.Until
			}
		}
	}
	if instanceID > 0 && until > byID[instanceID] {
		byID[instanceID] = until
	}
	result := make([]guestShopInstanceProtection, 0, len(byID))
	for id, protectedUntil := range byID {
		result = append(result, guestShopInstanceProtection{InstanceID: id, Until: protectedUntil})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].InstanceID < result[j].InstanceID })
	return result
}

func (s *PaymentConfigService) guestShopProviderProtection(ctx context.Context, instanceID int64, now time.Time) (bool, int64, error) {
	if s == nil || s.settingRepo == nil || instanceID <= 0 {
		return false, 0, nil
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingGuestShopEnabled,
		SettingGuestShopStripeID,
		SettingGuestShopProtection,
	})
	if err != nil {
		return false, 0, fmt.Errorf("get guest shop provider protection: %w", err)
	}
	if values[SettingGuestShopEnabled] == "true" && pcParsePositiveInt64(values[SettingGuestShopStripeID]) == instanceID {
		return true, 0, nil
	}
	protections, err := parseGuestShopInstanceProtections(values[SettingGuestShopProtection])
	if err != nil {
		return false, 0, err
	}
	for _, protection := range protections {
		if protection.InstanceID == instanceID && protection.Until > now.Unix() {
			return true, protection.Until, nil
		}
	}
	return false, 0, nil
}

func (s *PaymentService) CreateGuestShopPayment(ctx context.Context, req CreateGuestShopPaymentRequest) (*GuestShopPayment, error) {
	// Anchor expiry before the external call so Stripe latency cannot extend the
	// route token beyond the old instance's switch grace period.
	routeExpiresAt := time.Now().Add(GuestShopRouteTokenTTL)
	if s == nil || s.configService == nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	settings, err := s.configService.guestShopSettings(ctx)
	if err != nil {
		slog.Error("guest shop settings lookup failed", "error", err)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	if !settings.Enabled {
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
	sel, err := s.configService.guestShopStripeInstance(ctx, settings.StripeInstanceID)
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
	shopRef, err := newGuestShopPaymentReference(settings.StripeInstanceID)
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
	checkoutToken, err := createGuestShopRouteToken(guestShopRouteClaims{
		Version:          guestShopRouteTokenVersion,
		InstanceID:       settings.StripeInstanceID,
		PaymentIntentID:  intentID,
		PaymentReference: shopRef,
		ExpiresAt:        routeExpiresAt.Unix(),
	}, sel.Config[guestShopStripeSecretKey])
	if err != nil {
		slog.Error("guest shop route token creation failed", "instance_id", sel.InstanceID, "error", err)
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "unable to start payment")
	}
	return &GuestShopPayment{
		Quote:           *quote,
		Amount:          quote.Total,
		Currency:        currency,
		ClientSecret:    pr.ClientSecret,
		PublishableKey:  publishableKey,
		PaymentIntentID: intentID,
		CheckoutToken:   checkoutToken,
	}, nil
}

func (s *PaymentService) GetGuestShopPaymentStatus(ctx context.Context, paymentIntentID, checkoutToken string) (*GuestShopPaymentStatus, error) {
	intentID := strings.TrimSpace(paymentIntentID)
	if len(intentID) > guestShopMaxIntentIDLength || !guestShopIntentIDRe.MatchString(intentID) {
		return nil, infraerrors.BadRequest("INVALID_PAYMENT_INTENT", "invalid payment intent")
	}
	if s == nil || s.configService == nil {
		return nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	sel, route, err := s.guestShopStatusStripeInstance(ctx, intentID, strings.TrimSpace(checkoutToken), time.Now())
	if err != nil {
		return nil, err
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
	if result == nil || strings.TrimSpace(result.TradeNo) != intentID || !isGuestShopStripeResult(result, route) {
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

func (s *PaymentService) guestShopStatusStripeInstance(ctx context.Context, intentID, checkoutToken string, now time.Time) (*payment.InstanceSelection, *guestShopRouteClaims, error) {
	if checkoutToken == "" {
		settings, err := s.configService.guestShopSettings(ctx)
		if err != nil {
			slog.Error("guest shop settings lookup failed", "error", err)
			return nil, nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
		}
		sel, err := s.configService.guestShopStripeInstance(ctx, settings.StripeInstanceID)
		if err != nil {
			slog.Error("guest shop Stripe config lookup failed", "error", err)
			return nil, nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
		}
		return sel, nil, nil
	}

	claims, payload, signature, err := decodeGuestShopRouteToken(checkoutToken, intentID, now)
	if err != nil {
		return nil, nil, err
	}
	sel, err := s.configService.guestShopStripeInstance(ctx, claims.InstanceID)
	if err != nil {
		slog.Error("guest shop routed Stripe config lookup failed", "instance_id", claims.InstanceID, "error", err)
		return nil, nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	if sel == nil {
		return nil, nil, infraerrors.ServiceUnavailable("PAYMENT_GATEWAY_ERROR", "payment is not configured")
	}
	if !verifyGuestShopRouteSignature(payload, signature, sel.Config[guestShopStripeSecretKey]) {
		return nil, nil, infraerrors.BadRequest("INVALID_GUEST_SHOP_PAYMENT_TOKEN", "checkout token is invalid")
	}
	return sel, claims, nil
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

func newGuestShopPaymentReference(instanceID int64) (string, error) {
	if instanceID <= 0 {
		return "", fmt.Errorf("guest shop payment reference requires a Stripe instance")
	}
	random := make([]byte, guestShopReferenceRandomSize)
	if _, err := cryptorand.Read(random); err != nil {
		return "", err
	}
	return guestShopPaymentRefV1Pref + strconv.FormatInt(instanceID, 10) + "_" + hex.EncodeToString(random), nil
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

func createGuestShopRouteToken(claims guestShopRouteClaims, secretKey string) (string, error) {
	if claims.Version != guestShopRouteTokenVersion || claims.InstanceID <= 0 ||
		!guestShopIntentIDRe.MatchString(claims.PaymentIntentID) ||
		claims.ExpiresAt <= 0 || strings.TrimSpace(secretKey) == "" {
		return "", fmt.Errorf("invalid guest shop route token claims")
	}
	referenceInstanceID, ok := guestShopPaymentReferenceInstanceID(claims.PaymentReference)
	if !ok || referenceInstanceID != claims.InstanceID {
		return "", fmt.Errorf("guest shop route token reference does not match instance")
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal guest shop route token: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	return payload + "." + signGuestShopRoutePayload(payload, secretKey), nil
}

func decodeGuestShopRouteToken(token, intentID string, now time.Time) (*guestShopRouteClaims, string, string, error) {
	if len(token) == 0 || len(token) > guestShopRouteTokenMaxLength {
		return nil, "", "", infraerrors.BadRequest("INVALID_GUEST_SHOP_PAYMENT_TOKEN", "checkout token is invalid")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, "", "", infraerrors.BadRequest("INVALID_GUEST_SHOP_PAYMENT_TOKEN", "checkout token is invalid")
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, "", "", infraerrors.BadRequest("INVALID_GUEST_SHOP_PAYMENT_TOKEN", "checkout token is invalid")
	}
	var claims guestShopRouteClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, "", "", infraerrors.BadRequest("INVALID_GUEST_SHOP_PAYMENT_TOKEN", "checkout token is invalid")
	}
	referenceInstanceID, referenceOK := guestShopPaymentReferenceInstanceID(claims.PaymentReference)
	if claims.Version != guestShopRouteTokenVersion || claims.InstanceID <= 0 ||
		claims.PaymentIntentID != intentID || !referenceOK || referenceInstanceID != claims.InstanceID ||
		claims.ExpiresAt <= now.Unix() {
		return nil, "", "", infraerrors.BadRequest("INVALID_GUEST_SHOP_PAYMENT_TOKEN", "checkout token is invalid or expired")
	}
	return &claims, parts[0], parts[1], nil
}

func signGuestShopRoutePayload(payload, secretKey string) string {
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(secretKey)))
	_, _ = mac.Write([]byte("sub2api:guest-shop-route:v1:"))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func verifyGuestShopRouteSignature(payload, signature, secretKey string) bool {
	if strings.TrimSpace(secretKey) == "" {
		return false
	}
	expected := signGuestShopRoutePayload(payload, secretKey)
	return hmac.Equal([]byte(signature), []byte(expected))
}

func isGuestShopStripeResult(result *payment.QueryOrderResponse, route *guestShopRouteClaims) bool {
	if result == nil || result.Metadata == nil {
		return false
	}
	reference := strings.TrimSpace(result.Metadata[guestShopStripeOrderIDMeta])
	if route != nil {
		return reference == route.PaymentReference
	}
	return isGuestShopPaymentReference(reference)
}

func isGuestShopPaymentReference(reference string) bool {
	reference = strings.TrimSpace(reference)
	return guestShopPaymentRefLegacyRe.MatchString(reference) || guestShopPaymentRefV1Re.MatchString(reference)
}

func guestShopPaymentReferenceInstanceID(reference string) (int64, bool) {
	matches := guestShopPaymentRefV1Re.FindStringSubmatch(strings.TrimSpace(reference))
	if len(matches) != 3 {
		return 0, false
	}
	instanceID, err := strconv.ParseInt(matches[1], 10, 64)
	return instanceID, err == nil && instanceID > 0
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
