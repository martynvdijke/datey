package repository

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	_ "github.com/mattn/go-sqlite3"
)

func newTestRelationshipRepo(t *testing.T) (*RelationshipRepository, *PersonRepository) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_rel?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewRelationshipRepository(client), NewPersonRepository(client)
}

func TestRelationshipSymmetricReadBack(t *testing.T) {
	relRepo, personRepo := newTestRelationshipRepo(t)
	ctx := context.Background()
	a, _ := personRepo.Create(ctx, "Alice Rel", "", "")
	b, _ := personRepo.Create(ctx, "Bob Rel", "", "")

	for _, typ := range []string{"partner", "sibling", "custom"} {
		label := ""
		if typ == "custom" {
			label = "godparent"
		}
		rel, err := relRepo.Add(ctx, a.ID, b.ID, typ, label)
		if err != nil {
			t.Fatalf("Add %s: %v", typ, err)
		}
		// Both sides should see it
		la, _ := relRepo.ListForPerson(ctx, a.ID)
		lb, _ := relRepo.ListForPerson(ctx, b.ID)
		if len(la) != 1 || len(lb) != 1 {
			t.Fatalf("expected 1 each for %s got %d %d", typ, len(la), len(lb))
		}
		// custom label precedence
		if typ == "custom" && la[0].EffectiveTypeLabel != "godparent" {
			t.Errorf("custom label not shown: %q", la[0].EffectiveTypeLabel)
		}
		if err := relRepo.Remove(ctx, rel.ID); err != nil {
			t.Fatalf("remove: %v", err)
		}
	}
}

func TestRelationshipParentInverse(t *testing.T) {
	relRepo, personRepo := newTestRelationshipRepo(t)
	ctx := context.Background()
	parent, _ := personRepo.Create(ctx, "Parent", "", "")
	child, _ := personRepo.Create(ctx, "Child", "", "")
	_, err := relRepo.Add(ctx, parent.ID, child.ID, "parent", "")
	if err != nil {
		t.Fatalf("Add parent: %v", err)
	}
	lp, _ := relRepo.ListForPerson(ctx, parent.ID)
	lc, _ := relRepo.ListForPerson(ctx, child.ID)
	// Spec: "Bob's page SHALL show Alice as a parent" — the label describes the
	// OTHER person's relation to the viewer. Parent's page shows the child as
	// "child"; child's page shows the parent as "parent".
	if lp[0].EffectiveTypeLabel != "child" {
		t.Errorf("parent side expected child got %q", lp[0].EffectiveTypeLabel)
	}
	if lc[0].EffectiveTypeLabel != "parent" {
		t.Errorf("child side expected parent got %q", lc[0].EffectiveTypeLabel)
	}
}

func TestRelationshipGuards(t *testing.T) {
	relRepo, personRepo := newTestRelationshipRepo(t)
	ctx := context.Background()
	a, _ := personRepo.Create(ctx, "A Guard", "", "")
	b, _ := personRepo.Create(ctx, "B Guard", "", "")
	if _, err := relRepo.Add(ctx, a.ID, a.ID, "partner", ""); err == nil {
		t.Error("self-link should be rejected")
	}
	_, _ = relRepo.Add(ctx, a.ID, b.ID, "parent", "")
	if _, err := relRepo.Add(ctx, b.ID, a.ID, "parent", ""); err == nil {
		t.Error("mutual parent should be rejected")
	}
	_, _ = relRepo.Add(ctx, a.ID, b.ID, "partner", "")
	if _, err := relRepo.Add(ctx, b.ID, a.ID, "partner", ""); err == nil {
		t.Error("duplicate symmetric should be rejected")
	}
	// remove works
	rels, _ := relRepo.ListForPerson(ctx, a.ID)
	for _, r := range rels {
		_ = relRepo.Remove(ctx, r.ID)
	}
	remaining, _ := relRepo.ListForPerson(ctx, a.ID)
	if len(remaining) != 0 {
		t.Errorf("expected 0 after remove, got %d", len(remaining))
	}
}

func TestRelationshipCustomLabelPrecedence(t *testing.T) {
	relRepo, personRepo := newTestRelationshipRepo(t)
	ctx := context.Background()
	a, _ := personRepo.Create(ctx, "A Custom", "", "")
	b, _ := personRepo.Create(ctx, "B Custom", "", "")
	rel, _ := relRepo.Add(ctx, a.ID, b.ID, "custom", "best friend")
	entries, _ := relRepo.ListForPerson(ctx, a.ID)
	if entries[0].EffectiveTypeLabel != "best friend" {
		t.Errorf("expected label 'best friend' got %q", entries[0].EffectiveTypeLabel)
	}
	// Also partner with label overrides
	_ = relRepo.Remove(ctx, rel.ID)
	_, _ = relRepo.Add(ctx, a.ID, b.ID, "partner", "spouse")
	entries, _ = relRepo.ListForPerson(ctx, a.ID)
	if entries[0].EffectiveTypeLabel != "spouse" {
		t.Errorf("label should override partner, got %q", entries[0].EffectiveTypeLabel)
	}
}
