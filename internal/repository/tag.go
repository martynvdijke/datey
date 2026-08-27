package repository

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/person"
	"github.com/datey/datey/ent/tag"
)

var tagNameRe = regexp.MustCompile(`^[a-z0-9_-]{1,30}$`)

// NormalizeTag trims and lowercases a tag name. Returns empty if invalid.
func NormalizeTag(name string) string {
	n := strings.TrimSpace(strings.ToLower(name))
	return n
}

// ValidateTagName checks normalized name against allowed pattern.
func ValidateTagName(name string) bool {
	return tagNameRe.MatchString(name)
}

type TagRepository struct {
	client *ent.Client
}

func NewTagRepository(client *ent.Client) *TagRepository {
	return &TagRepository{client: client}
}

func (r *TagRepository) FindOrCreate(ctx context.Context, rawName string) (*ent.Tag, error) {
	name := NormalizeTag(rawName)
	if name == "" || !ValidateTagName(name) {
		// Return error for invalid but allow caller to handle
		// Use generic error
		return nil, &InvalidTagError{Name: rawName}
	}
	existing, err := r.client.Tag.Query().Where(tag.Name(name)).Only(ctx)
	if err == nil {
		return existing, nil
	}
	if !ent.IsNotFound(err) {
		return nil, err
	}
	return r.client.Tag.Create().SetName(name).SetCreatedAt(time.Now()).Save(ctx)
}

type InvalidTagError struct{ Name string }

func (e *InvalidTagError) Error() string { return "invalid tag name: " + e.Name }

func (r *TagRepository) AddToPerson(ctx context.Context, personID int, rawName string) error {
	t, err := r.FindOrCreate(ctx, rawName)
	if err != nil {
		return err
	}
	return r.client.Person.UpdateOneID(personID).AddTagIDs(t.ID).Exec(ctx)
}

func (r *TagRepository) RemoveFromPerson(ctx context.Context, personID int, tagName string) error {
	name := NormalizeTag(tagName)
	t, err := r.client.Tag.Query().Where(tag.Name(name)).Only(ctx)
	if err != nil {
		return err
	}
	return r.client.Person.UpdateOneID(personID).RemoveTagIDs(t.ID).Exec(ctx)
}

func (r *TagRepository) ListByPerson(ctx context.Context, personID int) ([]*ent.Tag, error) {
	return r.client.Person.Query().Where(person.IDEQ(personID)).QueryTags().All(ctx)
}

func (r *TagRepository) SearchByPrefix(ctx context.Context, prefix string, limit int) ([]*ent.Tag, error) {
	norm := NormalizeTag(prefix)
	if limit <= 0 {
		limit = 10
	}
	q := r.client.Tag.Query().Order(ent.Asc(tag.FieldName)).Limit(limit)
	if norm != "" {
		q = q.Where(tag.NameHasPrefix(norm))
	}
	return q.All(ctx)
}

func (r *TagRepository) ListAll(ctx context.Context) ([]*ent.Tag, error) {
	return r.client.Tag.Query().Order(ent.Asc(tag.FieldName)).All(ctx)
}

// ListPeopleByTags returns people that have ALL of the given tag names (AND semantics).
func (r *TagRepository) ListPeopleByTags(ctx context.Context, tagNames []string) ([]*ent.Person, error) {
	if len(tagNames) == 0 {
		return nil, nil
	}
	// Normalize
	var names []string
	for _, n := range tagNames {
		norm := NormalizeTag(n)
		if norm != "" {
			names = append(names, norm)
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	// Use ent query: people where HasTagsWith(name in ...) for each tag = AND
	q := r.client.Person.Query()
	for _, n := range names {
		q = q.Where(person.HasTagsWith(tag.Name(n)))
	}
	return q.Order(ent.Asc(person.FieldName)).WithEvents().WithGroups().WithTags().All(ctx)
}
