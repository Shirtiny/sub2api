package repository

import (
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func planConcurrencyEntitlementToService(entitlement *dbent.SubscriptionConcurrencyEntitlement) (service.PlanConcurrencyEntitlement, bool) {
	if entitlement == nil || entitlement.Concurrency <= 0 {
		return service.PlanConcurrencyEntitlement{}, false
	}

	expiresAt := entitlement.ExpiresAt
	if subscription := entitlement.Edges.Subscription; subscription != nil {
		subscriptionExpiresAt := normalizeSubscriptionExpiresAt(subscription.ExpiresAt)
		if subscriptionExpiresAt.Before(expiresAt) {
			expiresAt = subscriptionExpiresAt
		}
		if subscription.CustomExpiresAt != nil && subscription.CustomExpiresAt.Before(expiresAt) {
			expiresAt = *subscription.CustomExpiresAt
		}
	}
	if !entitlement.StartsAt.Before(expiresAt) {
		return service.PlanConcurrencyEntitlement{}, false
	}

	return service.PlanConcurrencyEntitlement{
		SubscriptionID: entitlement.SubscriptionID,
		Concurrency:    entitlement.Concurrency,
		StartsAt:       entitlement.StartsAt,
		ExpiresAt:      expiresAt,
	}, true
}
