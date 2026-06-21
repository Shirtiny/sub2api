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

// CafeCoupon holds membership-benefit café checkout coupons.
type CafeCoupon struct {
	ent.Schema
}

func (CafeCoupon) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "cafe_coupons"},
	}
}

func (CafeCoupon) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").
			MaxLen(48).
			NotEmpty().
			Unique(),
		field.Int64("user_id"),
		field.Int("membership_level").
			Default(0),
		field.String("coupon_type").
			MaxLen(20),
		field.Float("value").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.String("period").
			MaxLen(20).
			Default("month"),
		field.Time("period_start").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("period_end").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("status").
			MaxLen(20).
			Default("issued"),
		field.Int64("order_id").
			Optional().
			Nillable(),
		field.Time("applied_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (CafeCoupon) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("cafe_coupons").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (CafeCoupon) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("code").Unique(),
		index.Fields("user_id"),
		index.Fields("status"),
		index.Fields("order_id"),
		index.Fields("user_id", "period", "period_start").Unique(),
	}
}
