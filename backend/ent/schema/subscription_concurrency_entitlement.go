package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SubscriptionConcurrencyEntitlement stores one time-bounded concurrency grant.
// A separate row is required because one user subscription can be extended by
// purchases with different concurrency values.
type SubscriptionConcurrencyEntitlement struct {
	ent.Schema
}

func (SubscriptionConcurrencyEntitlement) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_concurrency_entitlements"},
	}
}

func (SubscriptionConcurrencyEntitlement) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (SubscriptionConcurrencyEntitlement) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("subscription_id"),
		field.Int64("source_order_id").
			Optional().
			Nillable(),
		field.Int("concurrency").
			Positive().
			Max(2147483647).
			Comment("maximum user concurrency during this entitlement window"),
		field.Time("starts_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SubscriptionConcurrencyEntitlement) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("subscription_concurrency_entitlements").
			Field("user_id").
			Unique().
			Required(),
		edge.From("subscription", UserSubscription.Type).
			Ref("concurrency_entitlements").
			Field("subscription_id").
			Unique().
			Required(),
	}
}

func (SubscriptionConcurrencyEntitlement) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "starts_at", "expires_at"),
		index.Fields("subscription_id", "starts_at"),
		index.Fields("source_order_id").Unique(),
	}
}
