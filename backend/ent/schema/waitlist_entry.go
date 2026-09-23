package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// WaitlistEntry stores a public opt-in, not a user account or a verified identity.
type WaitlistEntry struct{ ent.Schema }

func (WaitlistEntry) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "waitlist_entries"}}
}

func (WaitlistEntry) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").MaxLen(254).NotEmpty().Unique().Immutable(),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("confirmation_attempted_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("confirmation_sent_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}
