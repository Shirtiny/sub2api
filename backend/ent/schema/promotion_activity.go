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

// PromotionActivity stores a time-bounded promotion definition.
type PromotionActivity struct {
	ent.Schema
}

func (PromotionActivity) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "promotion_activities"},
	}
}

func (PromotionActivity) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(100).NotEmpty(),
		field.String("activity_type").MaxLen(50).NotEmpty(),
		field.Bool("enabled").Default(false),
		field.Time("starts_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("ends_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int("max_uses_per_user").Positive().Max(1000),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (PromotionActivity) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("plan_bonuses", PromotionActivityPlan.Type),
		edge.To("participations", PromotionActivityParticipation.Type),
	}
}

func (PromotionActivity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("activity_type", "enabled", "starts_at", "ends_at"),
	}
}
