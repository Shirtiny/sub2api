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

// PromotionActivityPlan stores the per-plan benefit for an activity.
type PromotionActivityPlan struct {
	ent.Schema
}

func (PromotionActivityPlan) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "promotion_activity_plans"},
	}
}

func (PromotionActivityPlan) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("activity_id").Positive(),
		field.Int64("plan_id").Positive(),
		field.Int("bonus_days").Positive().Max(36500),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (PromotionActivityPlan) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("activity", PromotionActivity.Type).
			Ref("plan_bonuses").
			Field("activity_id").
			Unique().
			Required(),
	}
}

func (PromotionActivityPlan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("activity_id", "plan_id").Unique(),
		index.Fields("plan_id"),
	}
}
