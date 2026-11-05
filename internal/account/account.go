// Package account owns whole-account operations: exporting a member's data and
// closing their account. Both the HTML app and the JSON API call this one
// service so their behaviour cannot drift.
package account

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/apitoken"
	"github.com/datey/datey/ent/auditentry"
	"github.com/datey/datey/ent/notificationlog"
	"github.com/datey/datey/ent/passwordresettoken"
	"github.com/datey/datey/ent/person"
	"github.com/datey/datey/ent/pushsubscription"
	"github.com/datey/datey/ent/session"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/ent/usernotificationchannel"
)

var (
	// ErrLastAdmin means the account is the last admin and closing it would
	// leave the instance without anyone able to administer it.
	ErrLastAdmin = errors.New("the last admin cannot be deleted")
	// ErrNotFound means there is no account with the requested ID.
	ErrNotFound = errors.New("account not found")
	// ErrInvalid means the request was malformed before it reached the store.
	ErrInvalid = errors.New("invalid account request")
)

// DeletedActor is the placeholder written over audit entries that named the
// deleted account. It keeps the audit trail intact without retaining a
// username that could identify the person.
const DeletedActor = "deleted-user"

// Service performs whole-account operations.
type Service struct {
	Client *ent.Client
}

// New builds the shared account service.
func New(client *ent.Client) *Service {
	return &Service{Client: client}
}

// Delete removes the user row and every credential that could still act as the
// user: sessions, API tokens, password-reset tokens, push subscriptions and
// per-user notification channels. Rows that describe other people's data are
// untouched because Datey stores a shared address book, not per-user content.
// The last admin cannot be deleted. Every write happens in one transaction, so
// a failure leaves the account untouched.
func (s *Service) Delete(ctx context.Context, userID int) error {
	if userID <= 0 {
		return ErrInvalid
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	u, err := tx.User.Get(ctx, userID)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrNotFound
		}
		return err
	}

	if u.Role == user.RoleAdmin {
		others, err := tx.User.Query().
			Where(user.RoleEQ(user.RoleAdmin), user.IDNEQ(userID)).
			Count(ctx)
		if err != nil {
			return err
		}
		if others == 0 {
			return ErrLastAdmin
		}
	}

	if _, err := tx.Session.Delete().
		Where(session.HasUserWith(user.IDEQ(userID))).Exec(ctx); err != nil {
		return err
	}
	if _, err := tx.ApiToken.Delete().
		Where(apitoken.HasUserWith(user.IDEQ(userID))).Exec(ctx); err != nil {
		return err
	}
	if _, err := tx.PasswordResetToken.Delete().
		Where(passwordresettoken.HasUserWith(user.IDEQ(userID))).Exec(ctx); err != nil {
		return err
	}
	if _, err := tx.PushSubscription.Delete().
		Where(pushsubscription.HasUserWith(user.IDEQ(userID))).Exec(ctx); err != nil {
		return err
	}
	if _, err := tx.UserNotificationChannel.Delete().
		Where(usernotificationchannel.HasUserWith(user.IDEQ(userID))).Exec(ctx); err != nil {
		return err
	}
	// Notification history rows carry a user_id but no foreign key, so deleting
	// the user row would otherwise leave the identifier behind.
	if _, err := tx.NotificationLog.Delete().
		Where(notificationlog.UserIDEQ(userID)).Exec(ctx); err != nil {
		return err
	}
	// Audit entries keep the action and timestamp for security review, but the
	// username is replaced so the log no longer names the closed account.
	if _, err := tx.AuditEntry.Update().
		Where(auditentry.ActorUsernameEQ(u.Username)).
		SetActorUsername(DeletedActor).
		Save(ctx); err != nil {
		return err
	}
	// Some actions (for example a successful login) store the username in the
	// target column instead of the actor. Replace exact matches too, otherwise
	// the closed account would still be named in the audit trail.
	if _, err := tx.AuditEntry.Update().
		Where(auditentry.TargetEQ(u.Username)).
		SetTarget(fmt.Sprintf("user:%d", userID)).
		Save(ctx); err != nil {
		return err
	}

	if err := tx.User.DeleteOneID(userID).Exec(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

// Export is the JSON document returned by GET /api/v1/me/export.
//
// Datey's people, events, gift ideas, groups and notes belong to a shared
// address book rather than to individual accounts. Only data that is directly
// attributable to the requester is included: the account row, its preferences,
// credentials metadata, audit entries naming the account, and the person record
// the account explicitly linked itself to (with that person's own timeline,
// events, gifts, tags and relationships).
type Export struct {
	GeneratedAt          time.Time           `json:"generated_at"`
	Account              AccountExport       `json:"account"`
	LinkedPerson         *LinkedPersonExport `json:"linked_person,omitempty"`
	NotificationChannels []ChannelExport     `json:"notification_channels"`
	APITokens            []APITokenExport    `json:"api_tokens"`
	AuditEntries         []AuditEntryExport  `json:"audit_entries"`
	PushSubscriptions    int                 `json:"push_subscription_count"`
	Notes                []string            `json:"notes"`
}

type AccountExport struct {
	ID                    int       `json:"id"`
	Username              string    `json:"username"`
	Email                 *string   `json:"email,omitempty"`
	Role                  string    `json:"role"`
	OIDCLinked            bool      `json:"oidc_linked"`
	Locale                *string   `json:"locale,omitempty"`
	EinkMode              bool      `json:"eink_mode"`
	NotificationScopeMode string    `json:"notification_scope_mode"`
	NotificationScope     string    `json:"notification_scope_group_ids"`
	LinkedPersonID        *int      `json:"linked_person_id,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type ChannelExport struct {
	Type    string `json:"type"`
	Target  string `json:"target"`
	Enabled bool   `json:"enabled"`
}

type APITokenExport struct {
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type AuditEntryExport struct {
	CreatedAt time.Time `json:"created_at"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	SourceIP  string    `json:"source_ip"`
}

type LinkedPersonExport struct {
	ID              int                  `json:"id"`
	Name            string               `json:"name"`
	FirstName       *string              `json:"first_name,omitempty"`
	MiddleName      *string              `json:"middle_name,omitempty"`
	LastName        *string              `json:"last_name,omitempty"`
	Notes           string               `json:"notes"`
	Timezone        string               `json:"timezone"`
	ReminderDays    []int                `json:"reminder_days,omitempty"`
	NotifyBirthdays bool                 `json:"notify_birthdays"`
	Tags            []string             `json:"tags"`
	Groups          []string             `json:"groups"`
	Events          []PersonEventExport  `json:"events"`
	Timeline        []TimelineExport     `json:"timeline"`
	GiftIdeas       []GiftExport         `json:"gift_ideas"`
	Relationships   []RelationshipExport `json:"relationships"`
}

type PersonEventExport struct {
	Type        string    `json:"type"`
	Date        time.Time `json:"date"`
	Description string    `json:"description"`
	Notes       string    `json:"notes"`
}

type TimelineExport struct {
	Date time.Time `json:"date"`
	Note string    `json:"note"`
}

type GiftExport struct {
	Title      string    `json:"title"`
	Notes      string    `json:"notes"`
	PriceCents *int      `json:"price_cents,omitempty"`
	URL        string    `json:"url"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type RelationshipExport struct {
	Type  string  `json:"type"`
	Label *string `json:"label,omitempty"`
	Other string  `json:"other_person"`
}

// Export returns the requester's attributable data. Credential secrets are
// deliberately omitted: token hashes, push endpoints/keys and OIDC subjects
// cannot be handed back safely and are not needed to satisfy access requests.
func (s *Service) Export(ctx context.Context, userID int) (*Export, error) {
	if userID <= 0 {
		return nil, ErrInvalid
	}
	u, err := s.Client.User.Query().
		Where(user.IDEQ(userID)).
		WithNotificationChannels().
		WithAPITokens().
		WithPushSubscriptions().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	out := &Export{
		GeneratedAt: time.Now().UTC(),
		Account: AccountExport{
			ID:                    u.ID,
			Username:              u.Username,
			Email:                 u.Email,
			Role:                  string(u.Role),
			OIDCLinked:            u.OidcSub != nil && strings.TrimSpace(*u.OidcSub) != "",
			Locale:                u.Locale,
			EinkMode:              u.EinkMode,
			NotificationScopeMode: string(u.NotificationScopeMode),
			NotificationScope:     u.NotificationScopeGroupIds,
			LinkedPersonID:        u.LinkedPersonID,
			CreatedAt:             u.CreatedAt,
			UpdatedAt:             u.UpdatedAt,
		},
		PushSubscriptions:    len(u.Edges.PushSubscriptions),
		NotificationChannels: []ChannelExport{},
		APITokens:            []APITokenExport{},
		AuditEntries:         []AuditEntryExport{},
		Notes: []string{
			"The shared address book (people, events, gifts, groups and notes) is not attributed to individual accounts, so it is not included. Only the person record this account linked itself to is exported.",
			"API token secrets, push subscription endpoints and keys, password hashes and OIDC subject identifiers are deliberately omitted.",
		},
	}
	for _, c := range u.Edges.NotificationChannels {
		out.NotificationChannels = append(out.NotificationChannels, ChannelExport{
			Type:    c.ChannelType,
			Target:  c.Target,
			Enabled: c.Enabled,
		})
	}
	for _, t := range u.Edges.APITokens {
		out.APITokens = append(out.APITokens, APITokenExport{
			Name:       t.Name,
			CreatedAt:  t.CreatedAt,
			ExpiresAt:  t.ExpiresAt,
			RevokedAt:  t.RevokedAt,
			LastUsedAt: t.LastUsedAt,
		})
	}

	audits, err := s.Client.AuditEntry.Query().
		Where(auditentry.ActorUsernameEQ(u.Username)).
		Order(ent.Asc(auditentry.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range audits {
		out.AuditEntries = append(out.AuditEntries, AuditEntryExport{
			CreatedAt: a.CreatedAt,
			Action:    a.Action,
			Target:    a.Target,
			SourceIP:  a.SourceIP,
		})
	}

	if u.LinkedPersonID != nil {
		p, err := s.Client.Person.Query().
			Where(person.IDEQ(*u.LinkedPersonID)).
			WithEvents().
			WithTimeline().
			WithGiftIdeas().
			WithTags().
			WithGroups().
			WithOutgoingRelationships().
			WithIncomingRelationships().
			Only(ctx)
		if err != nil && !ent.IsNotFound(err) {
			return nil, err
		}
		if p != nil {
			out.LinkedPerson = exportPerson(p)
		}
	}
	return out, nil
}

func exportPerson(p *ent.Person) *LinkedPersonExport {
	out := &LinkedPersonExport{
		ID:              p.ID,
		Name:            p.Name,
		FirstName:       p.FirstName,
		MiddleName:      p.MiddleName,
		LastName:        p.LastName,
		Notes:           p.Notes,
		Timezone:        p.Timezone,
		ReminderDays:    p.ReminderDays,
		NotifyBirthdays: p.NotifyBirthdays,
		Tags:            []string{},
		Groups:          []string{},
		Events:          []PersonEventExport{},
		Timeline:        []TimelineExport{},
		GiftIdeas:       []GiftExport{},
		Relationships:   []RelationshipExport{},
	}
	for _, t := range p.Edges.Tags {
		out.Tags = append(out.Tags, t.Name)
	}
	for _, g := range p.Edges.Groups {
		out.Groups = append(out.Groups, g.Name)
	}
	for _, e := range p.Edges.Events {
		out.Events = append(out.Events, PersonEventExport{
			Type:        e.Type,
			Date:        e.Date,
			Description: e.Description,
			Notes:       e.Notes,
		})
	}
	for _, n := range p.Edges.Timeline {
		out.Timeline = append(out.Timeline, TimelineExport{Date: n.NoteDate, Note: n.Note})
	}
	for _, g := range p.Edges.GiftIdeas {
		out.GiftIdeas = append(out.GiftIdeas, GiftExport{
			Title:      g.Title,
			Notes:      g.Notes,
			PriceCents: g.PriceCents,
			URL:        g.URL,
			Status:     string(g.Status),
			CreatedAt:  g.CreatedAt,
			UpdatedAt:  g.UpdatedAt,
		})
	}
	for _, rel := range p.Edges.OutgoingRelationships {
		out.Relationships = append(out.Relationships, RelationshipExport{Type: string(rel.Type), Label: rel.Label, Other: "person #" + itoa(rel.ToID)})
	}
	for _, rel := range p.Edges.IncomingRelationships {
		out.Relationships = append(out.Relationships, RelationshipExport{Type: string(rel.Type), Label: rel.Label, Other: "person #" + itoa(rel.FromID)})
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
