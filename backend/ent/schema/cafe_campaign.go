package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"time"
)

// CafeCampaign is a public, presale-only code. Terms are immutable; only enabled can change.
type CafeCampaign struct{ ent.Schema }

func (CafeCampaign) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "cafe_campaigns"}}
}
func (CafeCampaign) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").MaxLen(48).NotEmpty().Unique().Immutable(),
		field.String("name").MaxRuneLen(100).NotEmpty().Immutable(),
		field.Int("discount_percent").Min(1).Max(99).Immutable(),
		field.Bool("enabled").Default(false),
		field.Time("starts_at").Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("expires_at").Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("created_by").Immutable(),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}
func (CafeCampaign) Indexes() []ent.Index { return []ent.Index{index.Fields("code").Unique()} }
