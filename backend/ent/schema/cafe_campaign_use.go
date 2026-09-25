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

// CafeCampaignUse keeps one durable slot per account/campaign. Unpaid attempts may
// be replaced; a paid/consumed slot is never restored by cancellation or refund.
type CafeCampaignUse struct{ ent.Schema }

func (CafeCampaignUse) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "cafe_campaign_uses"}}
}
func (CafeCampaignUse) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("campaign_id").Immutable(), field.Int64("user_id").Immutable(), field.Int64("order_id"),
		field.Time("used_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}
func (CafeCampaignUse) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("campaign_id", "user_id").Unique(), index.Fields("order_id").Unique(),
	}
}
