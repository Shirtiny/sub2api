package service

import (
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
)

func activeUserSubscriptionExpiresAt(now time.Time) predicate.UserSubscription {
	return usersubscription.Or(
		predicate.UserSubscription(sql.FieldIsNull(usersubscription.FieldExpiresAt)),
		usersubscription.ExpiresAtGT(now),
	)
}
