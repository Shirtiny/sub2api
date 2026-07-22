package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SubscriptionEarlyResetEntitlement stores the early-reset policy purchased
// for one subscription term. A user subscription may contain consecutive
// terms bought from plans with different policies.
type SubscriptionEarlyResetEntitlement struct {
	ent.Schema
}

func (SubscriptionEarlyResetEntitlement) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_early_reset_entitlements"},
	}
}

func (SubscriptionEarlyResetEntitlement) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (SubscriptionEarlyResetEntitlement) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("subscription_id"),
		field.Int64("source_order_id").Optional().Nillable(),
		field.Bool("enabled").Default(false),
		field.Int("duration_days").Default(0).Min(0).Max(36500),
		field.Bool("custom_term").Default(false),
		field.Time("starts_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("expires_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SubscriptionEarlyResetEntitlement) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("subscription_id", "custom_term", "starts_at", "expires_at"),
		index.Fields("source_order_id").Unique(),
	}
}
