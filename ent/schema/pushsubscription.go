package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// PushSubscription stores a browser Web Push subscription (RFC 8291) for a
// user. The endpoint is unique: re-subscribing with the same endpoint replaces
// the existing row. p256dh and auth are the base64url-encoded keys received
// from the browser's PushManager.subscribe() call.
type PushSubscription struct {
	ent.Schema
}

func (PushSubscription) Fields() []ent.Field {
	return []ent.Field{
		field.String("endpoint").Unique().NotEmpty(),
		field.String("p256dh").NotEmpty(),
		field.String("auth").NotEmpty(),
		field.Time("created_at"),
	}
}

func (PushSubscription) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("push_subscriptions").Unique(),
	}
}
