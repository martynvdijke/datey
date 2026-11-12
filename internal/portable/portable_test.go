package portable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/internal/repository"
	_ "github.com/mattn/go-sqlite3"
)

var testDBSeq int64

func newTestDeps(t *testing.T) (Deps, *ent.Client) {
	t.Helper()
	seq := atomic.AddInt64(&testDBSeq, 1)
	name := fmt.Sprintf("portable_%d", seq)
	client := enttest.Open(t, dialect.SQLite, fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", name))
	t.Cleanup(func() { _ = client.Close() })
	return depsFor(client), client
}

func depsFor(client *ent.Client) Deps {
	return Deps{
		People:        repository.NewPersonRepository(client),
		Groups:        repository.NewGroupRepository(client),
		Events:        repository.NewEventRepository(client),
		GiftIdeas:     repository.NewGiftIdeaRepository(client),
		Tags:          repository.NewTagRepository(client),
		Relationships: repository.NewRelationshipRepository(client),
		PersonNotes:   repository.NewPersonNoteRepository(client),
		GroupNotes:    repository.NewGroupNoteRepository(client),
		Rules:         repository.NewRecurringRuleRepository(client),
		Source:        Source{AppURL: "https://datey.example.com", AppVersion: "test"},
	}
}

// seedLibrary fills a fresh database with one record of every portable kind.
func seedLibrary(t *testing.T, deps Deps) {
	t.Helper()
	ctx := context.Background()

	alice, err := deps.People.CreateStructured(ctx, "Alice Smith", "Alice", "", "Smith", "Met at work", "")
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := deps.People.CreateStructured(ctx, "Bob Jones", "Bob", "", "Jones", "Neighbour", "")
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	group, err := deps.Groups.Create(ctx, "Friends", "Close friends")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := deps.Groups.AddPerson(ctx, group.ID, alice.ID); err != nil {
		t.Fatalf("add member: %v", err)
	}

	if _, err := deps.Tags.FindOrCreate(ctx, "vip"); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	for _, p := range []*ent.Person{alice, bob} {
		if err := deps.Tags.AddToPerson(ctx, p.ID, "vip"); err != nil {
			t.Fatalf("tag person: %v", err)
		}
	}

	event, err := deps.Events.CreateForPersonWithSchedule(ctx, alice.ID, "birthday",
		time.Date(1990, 5, 12, 0, 0, 0, 0, time.UTC), "Born in May", "gregorian",
		nil, nil, false, repository.NewEventSchedule("annual", 1, nil, nil))
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if _, err := deps.Events.SetNotes(ctx, event.ID, "likes cake"); err != nil {
		t.Fatalf("set event notes: %v", err)
	}
	if _, err := deps.Events.CreateForGroupWithSchedule(ctx, group.ID, "anniversary",
		time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC), "Group trip", "gregorian",
		nil, nil, false, repository.NewEventSchedule("annual", 1, nil, nil)); err != nil {
		t.Fatalf("create group event: %v", err)
	}

	gift, err := deps.GiftIdeas.Create(ctx, alice.ID, "Coffee grinder", "Burr grinder", intPtr(4999), "https://example.com/grinder")
	if err != nil {
		t.Fatalf("create gift: %v", err)
	}
	if _, err := deps.GiftIdeas.UpdateStatus(ctx, gift.ID, "purchased"); err != nil {
		t.Fatalf("gift status: %v", err)
	}

	if _, err := deps.Relationships.Add(ctx, alice.ID, bob.ID, "partner", "wife"); err != nil {
		t.Fatalf("create relationship: %v", err)
	}
	if _, err := deps.PersonNotes.Create(ctx, alice.ID, "Moved to Berlin", time.Date(2021, 3, 4, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("create person note: %v", err)
	}
	if _, err := deps.GroupNotes.Create(ctx, group.ID, "Annual reunion", time.Date(2022, 7, 8, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("create group note: %v", err)
	}
	if _, err := deps.Rules.Create(ctx, "Mother's Day", "nth_weekday", 2, 0, 5, 0); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	disabled, err := deps.Rules.Create(ctx, "Old rule", "fixed", 0, 0, 2, 14)
	if err != nil {
		t.Fatalf("create disabled rule: %v", err)
	}
	if _, err := deps.Rules.Update(ctx, disabled.ID, "Old rule", "fixed", 0, 0, 2, 14, false); err != nil {
		t.Fatalf("disable rule: %v", err)
	}
}

func intPtr(i int) *int { return &i }

func TestExportEnvelopeIdentityAndContent(t *testing.T) {
	deps, _ := newTestDeps(t)
	seedLibrary(t, deps)

	env, err := Export(context.Background(), deps)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if env.Format != Format || env.Version != Version {
		t.Fatalf("identity = %q/%d, want %q/%d", env.Format, env.Version, Format, Version)
	}
	if env.ExportedAt.IsZero() {
		t.Error("exported_at is zero")
	}
	if env.Source.AppURL != "https://datey.example.com" {
		t.Errorf("source app_url = %q", env.Source.AppURL)
	}
	if len(env.People) != 2 || len(env.Groups) != 1 || len(env.Tags) != 1 {
		t.Errorf("counts: people=%d groups=%d tags=%d", len(env.People), len(env.Groups), len(env.Tags))
	}
	if len(env.Events) != 2 || len(env.GiftIdeas) != 1 || len(env.Relationships) != 1 {
		t.Errorf("counts: events=%d gifts=%d rels=%d", len(env.Events), len(env.GiftIdeas), len(env.Relationships))
	}
	if len(env.PersonNotes) != 1 || len(env.GroupNotes) != 1 || len(env.RecurringRules) != 2 {
		t.Errorf("counts: person_notes=%d group_notes=%d rules=%d", len(env.PersonNotes), len(env.GroupNotes), len(env.RecurringRules))
	}
	if err := Validate(env); err != nil {
		t.Fatalf("exported envelope does not validate: %v", err)
	}
}

func TestRoundTripExportImport(t *testing.T) {
	ctx := context.Background()
	source, _ := newTestDeps(t)
	seedLibrary(t, source)
	env, err := Export(ctx, source)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	target, _ := newTestDeps(t)
	res, err := Import(ctx, target, env, Options{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.People.Created != 2 || res.Groups.Created != 1 || res.Tags.Created != 1 {
		t.Errorf("created people/groups/tags = %d/%d/%d", res.People.Created, res.Groups.Created, res.Tags.Created)
	}
	if res.Events.Created != 2 || res.GiftIdeas.Created != 1 || res.Relationships.Created != 1 {
		t.Errorf("created events/gifts/rels = %d/%d/%d", res.Events.Created, res.GiftIdeas.Created, res.Relationships.Created)
	}
	if res.PersonNotes.Created != 1 || res.GroupNotes.Created != 1 || res.RecurringRules.Created != 2 {
		t.Errorf("created notes/rules = %d/%d/%d", res.PersonNotes.Created, res.GroupNotes.Created, res.RecurringRules.Created)
	}

	rebuilt, err := Export(ctx, target)
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if !reflect.DeepEqual(env.People, rebuilt.People) {
		t.Errorf("people differ:\n%#v\n%#v", env.People, rebuilt.People)
	}
	if !reflect.DeepEqual(env.Groups, rebuilt.Groups) {
		t.Errorf("groups differ: %#v vs %#v", env.Groups, rebuilt.Groups)
	}
	if !reflect.DeepEqual(env.Tags, rebuilt.Tags) {
		t.Errorf("tags differ: %#v vs %#v", env.Tags, rebuilt.Tags)
	}
	if !reflect.DeepEqual(env.Events, rebuilt.Events) {
		t.Errorf("events differ:\n%#v\n%#v", env.Events, rebuilt.Events)
	}
	if !reflect.DeepEqual(env.GiftIdeas, rebuilt.GiftIdeas) {
		t.Errorf("gifts differ: %#v vs %#v", env.GiftIdeas, rebuilt.GiftIdeas)
	}
	if !reflect.DeepEqual(env.Relationships, rebuilt.Relationships) {
		t.Errorf("relationships differ: %#v vs %#v", env.Relationships, rebuilt.Relationships)
	}
	if !reflect.DeepEqual(env.PersonNotes, rebuilt.PersonNotes) {
		t.Errorf("person notes differ: %#v vs %#v", env.PersonNotes, rebuilt.PersonNotes)
	}
	if !reflect.DeepEqual(env.GroupNotes, rebuilt.GroupNotes) {
		t.Errorf("group notes differ: %#v vs %#v", env.GroupNotes, rebuilt.GroupNotes)
	}
	if !reflect.DeepEqual(env.RecurringRules, rebuilt.RecurringRules) {
		t.Errorf("rules differ: %#v vs %#v", env.RecurringRules, rebuilt.RecurringRules)
	}
}

func TestSecondImportIsNoOp(t *testing.T) {
	ctx := context.Background()
	source, _ := newTestDeps(t)
	seedLibrary(t, source)
	env, err := Export(ctx, source)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	target, _ := newTestDeps(t)
	if _, err := Import(ctx, target, env, Options{}); err != nil {
		t.Fatalf("first import: %v", err)
	}
	second, err := Import(ctx, target, env, Options{})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	created := []int{
		second.People.Created, second.Groups.Created, second.Tags.Created,
		second.Events.Created, second.GiftIdeas.Created, second.Relationships.Created,
		second.PersonNotes.Created, second.GroupNotes.Created, second.RecurringRules.Created,
	}
	for i, n := range created {
		if n != 0 {
			t.Errorf("second import created %d records for entity %d", n, i)
		}
	}
}

func TestPreviewCounts(t *testing.T) {
	ctx := context.Background()
	source, _ := newTestDeps(t)
	seedLibrary(t, source)
	env, err := Export(ctx, source)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	target, _ := newTestDeps(t)
	empty, err := LoadExisting(ctx, target)
	if err != nil {
		t.Fatalf("load empty: %v", err)
	}
	preview := Preview(empty, env)
	if preview.People.New != 2 || preview.Events.New != 2 || preview.GiftIdeas.New != 1 {
		t.Errorf("new counts = people %d events %d gifts %d", preview.People.New, preview.Events.New, preview.GiftIdeas.New)
	}
	if preview.People.Skip != 0 || preview.People.Conflict != 0 {
		t.Errorf("empty preview should have no skips/conflicts: %+v", preview.People)
	}

	if _, err := Import(ctx, target, env, Options{}); err != nil {
		t.Fatalf("import: %v", err)
	}
	loaded, err := LoadExisting(ctx, target)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	after := Preview(loaded, env)
	if after.People.New != 0 || after.People.Skip != 2 || after.People.Conflict != 0 {
		t.Errorf("after import people = %+v", after.People)
	}
	if after.Events.Skip != 2 || after.GiftIdeas.Skip != 1 || after.Relationships.Skip != 1 {
		t.Errorf("after import events/gifts/rels = %+v/%+v/%+v", after.Events, after.GiftIdeas, after.Relationships)
	}
	if after.RecurringRules.Skip != 2 {
		t.Errorf("after import rules = %+v", after.RecurringRules)
	}

	// A record that exists with different content is a conflict, not a skip.
	changed := env
	changed.People = append([]Person(nil), env.People...)
	for i := range changed.People {
		if changed.People[i].Name == "Alice Smith" {
			changed.People[i].Notes = "different"
		}
	}
	conflicted := Preview(loaded, changed)
	if conflicted.People.Conflict != 1 {
		t.Errorf("conflict count = %d, want 1", conflicted.People.Conflict)
	}
}

func TestParseRejectsNewerVersion(t *testing.T) {
	env := Envelope{Format: Format, Version: Version + 1}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := Parse(data); err == nil {
		t.Fatal("expected newer version to be rejected")
	} else if !strings.Contains(err.Error(), "version") {
		t.Errorf("error %q does not mention the version", err)
	}
}

func TestParseRejectsMalformedJSON(t *testing.T) {
	if _, err := Parse([]byte(`{"format": "datey-portable",`)); err == nil {
		t.Fatal("expected malformed JSON to be rejected")
	}
}

func TestValidateRejectsBadReferences(t *testing.T) {
	env := Envelope{
		Format:  Format,
		Version: Version,
		People:  []Person{{Name: "Alice"}},
		Events:  []Event{{Person: "Nobody", Type: "birthday", Date: "1990-05-12"}},
		GiftIdeas: []GiftIdea{
			{Person: "Alice", Title: "Book", Status: "lost", PriceCents: intPtr(-1)},
		},
	}
	err := Validate(env)
	if err == nil {
		t.Fatal("expected validation errors")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error is not a ValidationError: %v", err)
	}
	if len(ve.Problems) < 3 {
		t.Errorf("expected at least 3 problems, got %d: %v", len(ve.Problems), ve.Problems)
	}
}

func TestSecretsAbsentFromMarshal(t *testing.T) {
	ctx := context.Background()
	deps, client := newTestDeps(t)
	seedLibrary(t, deps)

	// Persist secret-adjacent internals that must never reach the document.
	people, err := deps.People.List(ctx)
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) == 0 {
		t.Fatal("no people seeded")
	}
	// The ent client is reachable through the repositories' queries; set the
	// internal fields through a dedicated update on the first person.
	if _, err := client.Person.UpdateOneID(people[0].ID).
		SetVcardData("BEGIN:VCARD\nFN:Alice\nEND:VCARD").
		SetImmichPersonID("immich-secret-id").
		SetCarddavUID("carddav-secret-uid").
		SetGoogleResourceName("people/google-secret").
		Save(ctx); err != nil {
		t.Fatalf("set internals: %v", err)
	}

	env, err := Export(ctx, deps)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := strings.ToLower(string(data))
	for _, banned := range []string{
		"vcard", "immich", "carddav", "google", "password", "token",
		"push_subscription", "session", "BEGIN:VCARD", "immich-secret-id",
	} {
		if strings.Contains(out, strings.ToLower(banned)) {
			t.Errorf("marshalled export contains %q", banned)
		}
	}
}
