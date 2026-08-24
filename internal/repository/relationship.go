package repository

import (
	"context"
	"strings"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/person"
	"github.com/datey/datey/ent/relationship"
)

// SelfLinkError is returned when a person is linked to themselves.
type SelfLinkError struct{}

func (e *SelfLinkError) Error() string { return "cannot relate a person to themselves" }

// ParentLoopError is returned for mutual parent loops.
type ParentLoopError struct{}

func (e *ParentLoopError) Error() string {
	return "cannot create parent relationship: these two people are already linked as parent and child in the opposite direction"
}

// DuplicateRelationshipError is returned when a duplicate symmetric link is attempted.
type DuplicateRelationshipError struct{}

func (e *DuplicateRelationshipError) Error() string { return "relationship already exists" }

// RelationshipEntry is normalized view for a person's relationships.
type RelationshipEntry struct {
	ID                 int
	OtherPersonID      int
	OtherPersonName    string
	EffectiveTypeLabel string
	Direction          string // "outgoing" or "incoming" raw; EffectiveTypeLabel already inverted for parent
	RawType            string
	RawLabel           *string
}

// RelationshipRepository handles person relationships.
type RelationshipRepository struct {
	client *ent.Client
}

func NewRelationshipRepository(client *ent.Client) *RelationshipRepository {
	return &RelationshipRepository{client: client}
}

// Add creates a relationship with guards.
func (r *RelationshipRepository) Add(ctx context.Context, fromID, toID int, relType, label string) (*ent.Relationship, error) {
	if fromID == toID {
		return nil, &SelfLinkError{}
	}
	relType = strings.TrimSpace(relType)
	label = strings.TrimSpace(label)
	var nillableLabel *string
	if label != "" {
		nillableLabel = &label
	}

	// Self-link already checked.

	// Parent-loop guard: reject if trying to add parent where opposite parent exists.
	if relType == "parent" {
		exists, err := r.client.Relationship.Query().
			Where(relationship.FromIDEQ(toID), relationship.ToIDEQ(fromID), relationship.TypeEQ(relationship.TypeParent)).
			Exist(ctx)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, &ParentLoopError{}
		}
	}

	// For symmetric types, check both directions for duplicate.
	if relType == "partner" || relType == "sibling" || relType == "custom" {
		typed := relationship.Type(relType)
		// Do explicit checks:
		e1, e := r.client.Relationship.Query().
			Where(relationship.FromIDEQ(fromID), relationship.ToIDEQ(toID), relationship.TypeEQ(typed)).
			Exist(ctx)
		if e != nil {
			return nil, e
		}
		if e1 {
			return nil, &DuplicateRelationshipError{}
		}
		e2, e := r.client.Relationship.Query().
			Where(relationship.FromIDEQ(toID), relationship.ToIDEQ(fromID), relationship.TypeEQ(typed)).
			Exist(ctx)
		if e != nil {
			return nil, e
		}
		if e2 {
			return nil, &DuplicateRelationshipError{}
		}
	} else {
		// For parent, also guard exact duplicate (from,to,type) - unique will catch but give friendly error.
		exists, err := r.client.Relationship.Query().
			Where(relationship.FromIDEQ(fromID), relationship.ToIDEQ(toID), relationship.TypeEQ(relationship.Type(relType))).
			Exist(ctx)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, &DuplicateRelationshipError{}
		}
	}

	// Map type string to enum setter
	builder := r.client.Relationship.Create().SetFromID(fromID).SetToID(toID)
	switch relType {
	case "partner":
		builder = builder.SetType(relationship.TypePartner)
	case "parent":
		builder = builder.SetType(relationship.TypeParent)
	case "sibling":
		builder = builder.SetType(relationship.TypeSibling)
	case "custom":
		builder = builder.SetType(relationship.TypeCustom)
	default:
		builder = builder.SetType(relationship.Type(relType))
	}
	if nillableLabel != nil {
		builder = builder.SetLabel(*nillableLabel)
	}
	return builder.Save(ctx)
}

func (r *RelationshipRepository) Remove(ctx context.Context, id int) error {
	return r.client.Relationship.DeleteOneID(id).Exec(ctx)
}

// ListForPerson returns normalized entries visible from personID.
func (r *RelationshipRepository) ListForPerson(ctx context.Context, personID int) ([]RelationshipEntry, error) {
	rels, err := r.client.Relationship.Query().
		Where(relationship.Or(relationship.FromIDEQ(personID), relationship.ToIDEQ(personID))).
		All(ctx)
	if err != nil {
		return nil, err
	}
	// Need person names for other side.
	// Collect other IDs
	idSet := make(map[int]bool)
	for _, rel := range rels {
		var other int
		if rel.FromID == personID {
			other = rel.ToID
		} else {
			other = rel.FromID
		}
		idSet[other] = true
	}
	// Fetch names
	names := make(map[int]string)
	if len(idSet) > 0 {
		ids := make([]int, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		persons, err := r.client.Person.Query().Where(person.IDIn(ids...)).All(ctx)
		if err != nil {
			return nil, err
		}
		for _, p := range persons {
			names[p.ID] = p.Name
		}
	}

	var out []RelationshipEntry
	for _, rel := range rels {
		isOutgoing := rel.FromID == personID
		otherID := rel.ToID
		if !isOutgoing {
			otherID = rel.FromID
		}
		var effective string
		switch {
		case rel.Label != nil && strings.TrimSpace(*rel.Label) != "":
			effective = strings.TrimSpace(*rel.Label)
		case rel.Type == relationship.TypeParent && isOutgoing:
			// Row stored parent→child and we are the parent: the OTHER person is our child.
			effective = "child"
		case rel.Type == relationship.TypeParent && !isOutgoing:
			// Row stored parent→child and we are the child: the OTHER person is our parent.
			effective = "parent"
		default:
			effective = string(rel.Type)
		}
		dir := "outgoing"
		if !isOutgoing {
			dir = "incoming"
		}
		out = append(out, RelationshipEntry{
			ID:                 rel.ID,
			OtherPersonID:      otherID,
			OtherPersonName:    names[otherID],
			EffectiveTypeLabel: effective,
			Direction:          dir,
			RawType:            string(rel.Type),
			RawLabel:           rel.Label,
		})
	}
	return out, nil
}
