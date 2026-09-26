package repository

import (
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func planConcurrencyEntitlementToService(entitlement *dbent.SubscriptionConcurrencyEntitlement) (service.PlanConcurrencyEntitlement, bool) {
	if entitlement == nil || entitlement.Concurrency <= 0 {
		return service.PlanConcurrencyEntitlement{}, false
	}

	startsAt, expiresAt := entitlement.StartsAt, entitlement.ExpiresAt
	if subscription := entitlement.Edges.Subscription; subscription != nil {
		if subscription.StartsAt.After(startsAt) {
			startsAt = subscription.StartsAt
		}
		subscriptionExpiresAt := normalizeSubscriptionExpiresAt(subscription.ExpiresAt)
		if subscriptionExpiresAt.Before(expiresAt) {
			expiresAt = subscriptionExpiresAt
		}
		if subscription.CustomExpiresAt != nil && subscription.CustomExpiresAt.Before(expiresAt) {
			expiresAt = *subscription.CustomExpiresAt
		}
	}
	if !startsAt.Before(expiresAt) {
		return service.PlanConcurrencyEntitlement{}, false
	}

	return service.PlanConcurrencyEntitlement{
		SubscriptionID: entitlement.SubscriptionID,
		Concurrency:    entitlement.Concurrency,
		StartsAt:       startsAt,
		ExpiresAt:      expiresAt,
	}, true
}
