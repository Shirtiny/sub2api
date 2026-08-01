package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PromotionActivityParticipation records one order's reservation or grant.
type PromotionActivityParticipation struct {
	ent.Schema
}

func (PromotionActivityParticipation) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "promotion_activity_participations"}}
}

func (PromotionActivityParticipation) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("activity_id").Positive(),
		field.Int64("user_id").Positive(),
		field.Int64("order_id").Positive(),
		field.Int64("plan_id").Positive(),
		field.Int("bonus_days").Positive().Max(36500),
		field.String("status").MaxLen(20),
		field.Time("reserved_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("granted_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("released_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("release_reason").Optional().Nillable().MaxLen(100),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (PromotionActivityParticipation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("activity", PromotionActivity.Type).
			Ref("participations").
			Field("activity_id").
			Unique().
			Required(),
	}
}

func (PromotionActivityParticipation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("order_id").Unique(),
		index.Fields("activity_id", "user_id", "status"),
		index.Fields("user_id", "status"),
	}
}
