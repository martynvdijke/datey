package account

import (
	"context"
	"errors"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/apitoken"
	"github.com/datey/datey/ent/auditentry"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/passwordresettoken"
	"github.com/datey/datey/ent/person"
	"github.com/datey/datey/ent/pushsubscription"
	"github.com/datey/datey/ent/session"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/ent/usernotificationchannel"
	_ "github.com/mattn/go-sqlite3"
)

func newAccountTestClient(t *testing.T) *ent.Client {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_account_service?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return client
}

func seedAccountUser(t *testing.T, client *ent.Client, username string, role user.Role) *ent.User {
	t.Helper()
	return client.User.Create().
		SetUsername(username).
		SetPasswordHash("hash").
		SetRole(role).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		SaveX(context.Background())
}

func TestDeleteRevokesCredentialsAndKeepsSharedData(t *testing.T) {
	client := newAccountTestClient(t)
	svc := New(client)
	u := seedAccountUser(t, client, "alice", user.RoleUser)
	admin := seedAccountUser(t, client, "admin", user.RoleAdmin)
	_ = admin

	now := time.Now()
	client.Session.Create().
		SetTokenHash("session-hash").
		SetUserID(u.ID).
		SetExpiresAt(now.Add(time.Hour)).
		SetCreatedAt(now).
		SaveX(context.Background())
	client.ApiToken.Create().
		SetTokenHash("token-hash").
		SetName("laptop").
		SetUserID(u.ID).
		SetCreatedAt(now).
		SaveX(context.Background())
	client.PasswordResetToken.Create().
		SetTokenHash("reset-hash").
		SetUserID(u.ID).
		SetExpiresAt(now.Add(time.Hour)).
		SetCreatedAt(now).
		SaveX(context.Background())
	client.PushSubscription.Create().
		SetEndpoint("https://push.example.test/1").
		SetP256dh("key").
		SetAuth("auth").
		SetUserID(u.ID).
		SetCreatedAt(now).
		SaveX(context.Background())
	client.UserNotificationChannel.Create().
		SetChannelType("ntfy").
		SetTarget("topic").
		SetUserID(u.ID).
		SaveX(context.Background())
	client.AuditEntry.Create().
		SetCreatedAt(now).
		SetActorUsername("alice").
		SetAction("login").
		SetTarget("web").
		SetSourceIP("127.0.0.1").
		SaveX(context.Background())
	sharedPerson := client.Person.Create().
		SetName("Shared Person").
		SetCreatedAt(now).
		SetUpdatedAt(now).
		SaveX(context.Background())

	if err := svc.Delete(context.Background(), u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	ctx := context.Background()
	if client.User.Query().Where(user.IDEQ(u.ID)).ExistX(ctx) {
		t.Fatal("user row still exists")
	}
	for name, q := range map[string]bool{
		"session":              client.Session.Query().Where(session.HasUserWith(user.IDEQ(u.ID))).ExistX(ctx),
		"api token":            client.ApiToken.Query().Where(apitoken.HasUserWith(user.IDEQ(u.ID))).ExistX(ctx),
		"reset token":          client.PasswordResetToken.Query().Where(passwordresettoken.HasUserWith(user.IDEQ(u.ID))).ExistX(ctx),
		"push subscription":    client.PushSubscription.Query().Where(pushsubscription.HasUserWith(user.IDEQ(u.ID))).ExistX(ctx),
		"notification channel": client.UserNotificationChannel.Query().Where(usernotificationchannel.HasUserWith(user.IDEQ(u.ID))).ExistX(ctx),
	} {
		if q {
			t.Fatalf("%s still exists", name)
		}
	}
	if !client.Person.Query().Where(person.IDEQ(sharedPerson.ID)).ExistX(ctx) {
		t.Fatal("shared address-book person was deleted")
	}
	audit := client.AuditEntry.Query().Order(ent.Asc(auditentry.FieldID)).FirstX(ctx)
	if audit.ActorUsername != DeletedActor {
		t.Fatalf("audit actor %q, want %q", audit.ActorUsername, DeletedActor)
	}
}

func TestDeleteRefusesLastAdmin(t *testing.T) {
	client := newAccountTestClient(t)
	svc := New(client)
	admin := seedAccountUser(t, client, "admin", user.RoleAdmin)

	err := svc.Delete(context.Background(), admin.ID)
	if !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("err %v, want ErrLastAdmin", err)
	}
	if !client.User.Query().Where(user.IDEQ(admin.ID)).ExistX(context.Background()) {
		t.Fatal("last admin was deleted")
	}
}

func TestDeleteAllowsAdminWhenAnotherRemains(t *testing.T) {
	client := newAccountTestClient(t)
	svc := New(client)
	first := seedAccountUser(t, client, "admin-one", user.RoleAdmin)
	seedAccountUser(t, client, "admin-two", user.RoleAdmin)

	if err := svc.Delete(context.Background(), first.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestDeleteUnknownUser(t *testing.T) {
	client := newAccountTestClient(t)
	svc := New(client)
	if err := svc.Delete(context.Background(), 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err %v, want ErrNotFound", err)
	}
	if err := svc.Delete(context.Background(), 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err %v, want ErrInvalid", err)
	}
}

func TestExportIncludesLinkedPersonAndOmitsSecrets(t *testing.T) {
	client := newAccountTestClient(t)
	svc := New(client)
	u := seedAccountUser(t, client, "alice", user.RoleUser)
	now := time.Now()
	p := client.Person.Create().
		SetName("Alice Person").
		SetNotes("own record").
		SetCreatedAt(now).
		SetUpdatedAt(now).
		SaveX(context.Background())
	client.User.UpdateOneID(u.ID).SetLinkedPersonID(p.ID).ExecX(context.Background())
	client.ApiToken.Create().
		SetTokenHash("super-secret-hash").
		SetName("laptop").
		SetUserID(u.ID).
		SetCreatedAt(now).
		SaveX(context.Background())

	export, err := svc.Export(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if export.Account.Username != "alice" {
		t.Fatalf("username %q", export.Account.Username)
	}
	if export.LinkedPerson == nil || export.LinkedPerson.Name != "Alice Person" {
		t.Fatalf("linked person missing: %+v", export.LinkedPerson)
	}
	if len(export.APITokens) != 1 || export.APITokens[0].Name != "laptop" {
		t.Fatalf("api token metadata missing: %+v", export.APITokens)
	}
}
