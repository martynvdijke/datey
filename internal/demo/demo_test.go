package demo

import (
	"context"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/repository"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

func newDemoTestClient(t *testing.T) *ent.Client {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	client := enttest.Open(t, dialect.SQLite, "file:"+name+"?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return client
}

func demoConfig() *config.Config {
	return &config.Config{
		DemoUserEnabled:  true,
		DemoUserHandle:   "demo",
		DemoUserPassword: "demo-pass-123",
		DemoUserEmail:    "demo@datey.local",
	}
}

func runEnsure(t *testing.T, client *ent.Client, cfg *config.Config) error {
	t.Helper()
	return Ensure(context.Background(), client, cfg, repository.NewUserRepository(client))
}

func TestEnsure_CreatesDemoUserAndSeedsContent(t *testing.T) {
	client := newDemoTestClient(t)
	cfg := demoConfig()

	if err := runEnsure(t, client, cfg); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	u, err := client.User.Query().Where(user.UsernameEQ("demo")).Only(context.Background())
	if err != nil {
		t.Fatalf("find demo user: %v", err)
	}
	if u.Role != user.RoleUser {
		t.Errorf("role = %q, want user", u.Role)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(cfg.DemoUserPassword)); err != nil {
		t.Errorf("password hash does not match configured password: %v", err)
	}
	if u.Email == nil || *u.Email != cfg.DemoUserEmail {
		t.Errorf("email = %v, want %q", u.Email, cfg.DemoUserEmail)
	}
	if u.LinkedPersonID == nil {
		t.Error("expected demo user to be linked to a person")
	}

	assertCount(t, "people", mustCount(t, client.Person.Query()), 3)
	assertCount(t, "events", mustCount(t, client.Event.Query()), 4)
	assertCount(t, "gifts", mustCount(t, client.GiftIdea.Query()), 2)
	assertCount(t, "notes", mustCount(t, client.PersonNote.Query()), 2)
	if n := mustCount(t, client.Group.Query()); n != 1 {
		t.Errorf("groups = %d, want 1", n)
	}
	if n := mustCount(t, client.Tag.Query()); n != 2 {
		t.Errorf("tags = %d, want 2", n)
	}
	assertCount(t, "relationships", mustCount(t, client.Relationship.Query()), 3)
}

func TestEnsure_DisabledIsNoOp(t *testing.T) {
	client := newDemoTestClient(t)
	cfg := demoConfig()
	cfg.DemoUserEnabled = false

	if err := runEnsure(t, client, cfg); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if n := mustCount(t, client.User.Query()); n != 0 {
		t.Errorf("users = %d, want 0", n)
	}
	if n := mustCount(t, client.Person.Query()); n != 0 {
		t.Errorf("people = %d, want 0", n)
	}
}

func TestEnsure_Idempotent(t *testing.T) {
	client := newDemoTestClient(t)
	cfg := demoConfig()

	if err := runEnsure(t, client, cfg); err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	firstPeople := mustCount(t, client.Person.Query())
	if err := runEnsure(t, client, cfg); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}

	if n := mustCount(t, client.User.Query()); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}
	if n := mustCount(t, client.Person.Query()); n != firstPeople {
		t.Errorf("people = %d, want %d", n, firstPeople)
	}
	if n := mustCount(t, client.Event.Query()); n != 4 {
		t.Errorf("events = %d, want 4", n)
	}
	if n := mustCount(t, client.GiftIdea.Query()); n != 2 {
		t.Errorf("gifts = %d, want 2", n)
	}
	if n := mustCount(t, client.PersonNote.Query()); n != 2 {
		t.Errorf("notes = %d, want 2", n)
	}
	if n := mustCount(t, client.Relationship.Query()); n != 3 {
		t.Errorf("relationships = %d, want 3", n)
	}
}

func TestEnsure_RefusesDifferentEmailTakeover(t *testing.T) {
	client := newDemoTestClient(t)
	cfg := demoConfig()
	ctx := context.Background()
	now := time.Now()

	original, err := bcrypt.GenerateFromPassword([]byte("original-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	existing, err := client.User.Create().
		SetUsername("demo").
		SetPasswordHash(string(original)).
		SetEmail("someone-else@example.com").
		SetRole(user.RoleUser).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed existing user: %v", err)
	}

	if err := runEnsure(t, client, cfg); err == nil {
		t.Fatal("expected Ensure to refuse a different-email takeover")
	}

	got, err := client.User.Get(ctx, existing.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if got.Email == nil || *got.Email != "someone-else@example.com" {
		t.Errorf("email = %v, want unchanged", got.Email)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(got.PasswordHash), []byte("original-password")); err != nil {
		t.Errorf("password was changed despite takeover refusal: %v", err)
	}
}

func TestEnsure_RefusesAdminTakeover(t *testing.T) {
	client := newDemoTestClient(t)
	cfg := demoConfig()
	ctx := context.Background()
	now := time.Now()

	original, err := bcrypt.GenerateFromPassword([]byte("admin-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	existing, err := client.User.Create().
		SetUsername("demo").
		SetPasswordHash(string(original)).
		SetEmail(cfg.DemoUserEmail).
		SetRole(user.RoleAdmin).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed admin user: %v", err)
	}

	if err := runEnsure(t, client, cfg); err == nil {
		t.Fatal("expected Ensure to refuse an admin account")
	}

	got, err := client.User.Get(ctx, existing.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if got.Role != user.RoleAdmin {
		t.Errorf("role = %q, want admin (must not demote)", got.Role)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(got.PasswordHash), []byte("admin-password")); err != nil {
		t.Errorf("admin password was changed: %v", err)
	}
}

func TestEnsure_DoesNotSeedWhenPeopleExist(t *testing.T) {
	client := newDemoTestClient(t)
	cfg := demoConfig()
	ctx := context.Background()
	now := time.Now()

	if _, err := client.Person.Create().
		SetName("Existing Person").
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		t.Fatalf("seed existing person: %v", err)
	}

	if err := runEnsure(t, client, cfg); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if n := mustCount(t, client.Person.Query()); n != 1 {
		t.Errorf("people = %d, want 1 (existing only)", n)
	}
	if n := mustCount(t, client.Event.Query()); n != 0 {
		t.Errorf("events = %d, want 0", n)
	}
	if n := mustCount(t, client.GiftIdea.Query()); n != 0 {
		t.Errorf("gifts = %d, want 0", n)
	}
	if n := mustCount(t, client.PersonNote.Query()); n != 0 {
		t.Errorf("notes = %d, want 0", n)
	}
	if n := mustCount(t, client.Group.Query()); n != 0 {
		t.Errorf("groups = %d, want 0", n)
	}
}

func TestEnsure_RejectsInvalidConfig(t *testing.T) {
	client := newDemoTestClient(t)

	cases := []struct {
		name string
		edit func(*config.Config)
	}{
		{"short handle", func(c *config.Config) { c.DemoUserHandle = "ab" }},
		{"invalid handle", func(c *config.Config) { c.DemoUserHandle = "bad handle" }},
		{"empty password", func(c *config.Config) { c.DemoUserPassword = "" }},
		{"invalid email", func(c *config.Config) { c.DemoUserEmail = "not-an-email" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := demoConfig()
			tc.edit(cfg)
			if err := runEnsure(t, client, cfg); err == nil {
				t.Errorf("expected validation error for %s", tc.name)
			}
		})
	}
}

type counter interface {
	Count(context.Context) (int, error)
}

func mustCount(t *testing.T, c counter) int {
	t.Helper()
	n, err := c.Count(context.Background())
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func assertCount(t *testing.T, name string, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %d, want %d", name, got, want)
	}
}
