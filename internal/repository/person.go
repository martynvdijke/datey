package repository

import (
	"context"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/giftidea"
	"github.com/datey/datey/ent/group"
	"github.com/datey/datey/ent/person"
	"github.com/datey/datey/ent/relationship"
)

type PersonRepository struct {
	client *ent.Client
}

func NewPersonRepository(client *ent.Client) *PersonRepository {
	return &PersonRepository{client: client}
}

func (r *PersonRepository) Create(ctx context.Context, name, notes, vcardData string) (*ent.Person, error) {
	return r.CreateStructured(ctx, name, "", "", "", notes, vcardData)
}

// CreateStructured persists a person with both the computed display name and
// its structured components. Empty structured parts are stored as NULL so
// legacy rows and new display-name-only rows are indistinguishable.
func (r *PersonRepository) CreateStructured(ctx context.Context, display, first, middle, last, notes, vcardData string) (*ent.Person, error) {
	mutation := r.client.Person.Create().
		SetName(display).
		SetNillableFirstName(nillableStrOrNil(first)).
		SetNillableMiddleName(nillableStrOrNil(middle)).
		SetNillableLastName(nillableStrOrNil(last)).
		SetNotes(notes).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now())
	if vcardData != "" {
		mutation = mutation.SetVcardData(vcardData)
	}
	return mutation.Save(ctx)
}

func (r *PersonRepository) Get(ctx context.Context, id int) (*ent.Person, error) {
	return r.client.Person.Get(ctx, id)
}

func (r *PersonRepository) List(ctx context.Context) ([]*ent.Person, error) {
	return r.client.Person.Query().
		Order(ent.Asc(person.FieldName)).
		WithEvents().
		WithGroups().
		WithTags().
		All(ctx)
}

func (r *PersonRepository) Search(ctx context.Context, q string) ([]*ent.Person, error) {
	return r.client.Person.Query().
		Where(person.NameContainsFold(q)).
		Order(ent.Asc(person.FieldName)).
		WithEvents().
		WithGroups().
		WithTags().
		All(ctx)
}

// ListByGroupIDs returns the de-duplicated union of members of the given
// group IDs in a single query.
func (r *PersonRepository) ListByGroupIDs(ctx context.Context, groupIDs []int) ([]*ent.Person, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	return r.client.Person.Query().
		Where(person.HasGroupsWith(group.IDIn(groupIDs...))).
		Order(ent.Asc(person.FieldName)).
		WithEvents().
		WithGroups().
		WithTags().
		All(ctx)
}

func (r *PersonRepository) Update(ctx context.Context, id int, name, notes, vcardData string) (*ent.Person, error) {
	return r.UpdateStructured(ctx, id, name, "", "", "", notes, vcardData)
}

// UpdateStructured saves the display name and structured name components.
// Empty parts clear their nullable columns so removing a middle name works;
// the display name must stay non-empty (unique constraint applies).
// applyNamePart sets or clears one structured name column on the update
// builder: empty clears the nullable column (so removing a middle name
// works), non-empty sets it.
func applyNamePart[T any](set func(string) *T, clear func() *T, value string) {
	if value != "" {
		set(value)
		return
	}
	clear()
}

func (r *PersonRepository) UpdateStructured(ctx context.Context, id int, display, first, middle, last, notes, vcardData string) (*ent.Person, error) {
	mutation := r.client.Person.UpdateOneID(id).
		SetName(display).
		SetNotes(notes).
		SetUpdatedAt(time.Now())
	applyNamePart(mutation.SetFirstName, mutation.ClearFirstName, first)
	applyNamePart(mutation.SetMiddleName, mutation.ClearMiddleName, middle)
	applyNamePart(mutation.SetLastName, mutation.ClearLastName, last)
	if vcardData != "" {
		mutation = mutation.SetVcardData(vcardData)
	}
	// Local edits to a synced person must be pushed back to the remote
	// address book on the next sync; mark it dirty when it has a UID.
	existing, err := r.client.Person.Get(ctx, id)
	if err == nil && existing.CarddavUID != nil {
		mutation = mutation.SetCarddavPendingSync(true)
	}
	return mutation.Save(ctx)
}

func (r *PersonRepository) FindByName(ctx context.Context, name string) (*ent.Person, error) {
	return r.client.Person.Query().
		Where(person.Name(name)).
		Only(ctx)
}

// FindByCarddavUID looks up a person by the UID assigned by the remote
// CardDAV address book. Returns ent.NotFoundError when no match exists.
func (r *PersonRepository) FindByCarddavUID(ctx context.Context, uid string) (*ent.Person, error) {
	return r.client.Person.Query().
		Where(person.CarddavUID(uid)).
		Only(ctx)
}

// FindByCarddavHref looks up a person by the remote resource URL. Used when a
// sync report reports a deletion by href. Returns ent.NotFoundError when no
// match exists.
func (r *PersonRepository) FindByCarddavHref(ctx context.Context, href string) (*ent.Person, error) {
	return r.client.Person.Query().
		Where(person.CarddavHref(href)).
		Only(ctx)
}

// ListForCarddavPush returns the people that need to be pushed to the remote
// address book: locally created contacts without a UID yet (pending) and
// synced contacts with local changes (pending flag set).
func (r *PersonRepository) ListForCarddavPush(ctx context.Context) ([]*ent.Person, error) {
	return r.client.Person.Query().
		Where(person.CarddavPendingSync(true)).
		Order(ent.Asc(person.FieldName)).
		All(ctx)
}

// ListCarddavSynced returns all people that have been synced at least once
// (have a remote UID), used for remote-deletion reconciliation.
func (r *PersonRepository) ListCarddavSynced(ctx context.Context) ([]*ent.Person, error) {
	return r.client.Person.Query().
		Where(person.CarddavUIDNotNil()).
		Order(ent.Asc(person.FieldName)).
		All(ctx)
}

// SetCarddavState records the sync bookkeeping returned by the remote server
// after a successful push or pull. pendingSync marks whether a local change
// still needs to be pushed.
func (r *PersonRepository) SetCarddavState(ctx context.Context, id int, uid, href, etag, rev string, lastModified *time.Time, pendingSync bool) (*ent.Person, error) {
	return r.client.Person.UpdateOneID(id).
		SetNillableCarddavUID(nillableStrOrNil(uid)).
		SetNillableCarddavHref(nillableStrOrNil(href)).
		SetNillableCarddavEtag(nillableStrOrNil(etag)).
		SetNillableCarddavRev(nillableStrOrNil(rev)).
		SetNillableCarddavLastModified(lastModified).
		SetCarddavPendingSync(pendingSync).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

// SetCarddavPendingSync marks whether a person has unsynced local changes.
func (r *PersonRepository) SetCarddavPendingSync(ctx context.Context, id int, pending bool) (*ent.Person, error) {
	return r.client.Person.UpdateOneID(id).
		SetCarddavPendingSync(pending).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

// ClearCarddavState removes the CardDAV linkage from a person while keeping
// the person itself. Used when the remote resource is deleted and the
// configured policy is "keep": the contact stays local but is no longer synced.
func (r *PersonRepository) ClearCarddavState(ctx context.Context, id int) (*ent.Person, error) {
	return r.client.Person.UpdateOneID(id).
		ClearCarddavUID().
		ClearCarddavHref().
		ClearCarddavEtag().
		ClearCarddavRev().
		ClearCarddavLastModified().
		SetCarddavPendingSync(false).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

func (r *PersonRepository) UpdateCarddavFields(ctx context.Context, id int, name, notes, vcardData string) (*ent.Person, error) {
	return r.UpdateCarddavFieldsStructured(ctx, id, name, "", "", "", notes, vcardData)
}

// UpdateCarddavFieldsStructured applies remote-sourced field changes
// (structured name/notes/vCard payload) without marking the person as pending
// — the remote is authoritative during a pull, so the change must not be
// echoed back.
func (r *PersonRepository) UpdateCarddavFieldsStructured(ctx context.Context, id int, display, first, middle, last, notes, vcardData string) (*ent.Person, error) {
	mutation := r.client.Person.UpdateOneID(id).
		SetName(display).
		SetNotes(notes).
		SetUpdatedAt(time.Now())
	applyNamePart(mutation.SetFirstName, mutation.ClearFirstName, first)
	applyNamePart(mutation.SetMiddleName, mutation.ClearMiddleName, middle)
	applyNamePart(mutation.SetLastName, mutation.ClearLastName, last)
	if vcardData != "" {
		mutation = mutation.SetVcardData(vcardData)
	}
	return mutation.Save(ctx)
}

// SetNotifyBirthdays updates the per-person birthday notification opt-out.
func (r *PersonRepository) SetNotifyBirthdays(ctx context.Context, id int, enabled bool) (*ent.Person, error) {
	return r.client.Person.UpdateOneID(id).
		SetNotifyBirthdays(enabled).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

func (r *PersonRepository) SetImmichPhoto(ctx context.Context, id int, immichID *string, disabled bool) (*ent.Person, error) {
	return r.client.Person.UpdateOneID(id).
		SetNillableImmichPersonID(immichID).
		SetImmichPhotoDisabled(disabled).
		SetUpdatedAt(time.Now()).Save(ctx)
}

// SetPhotoState records a locally stored profile photo for a person.
// photoPath is relative to the photos directory; source is "immich" or "upload".
func (r *PersonRepository) SetPhotoState(ctx context.Context, id int, photoPath, contentType, source string) (*ent.Person, error) {
	now := time.Now()
	return r.client.Person.UpdateOneID(id).
		SetPhotoPath(photoPath).
		SetPhotoContentType(contentType).
		SetPhotoUpdatedAt(now).
		SetPhotoSource(source).
		SetUpdatedAt(now).
		Save(ctx)
}

// ClearPhoto removes all local photo state so the person reverts to the
// proxy/fallback behavior.
func (r *PersonRepository) ClearPhoto(ctx context.Context, id int) (*ent.Person, error) {
	now := time.Now()
	return r.client.Person.UpdateOneID(id).
		ClearPhotoPath().
		ClearPhotoContentType().
		ClearPhotoUpdatedAt().
		ClearPhotoSource().
		SetUpdatedAt(now).
		Save(ctx)
}

func (r *PersonRepository) Delete(ctx context.Context, id int) error {
	// Cascade: delete gift ideas owned by this person before deleting the person
	// to avoid FK violations.
	if _, err := r.client.GiftIdea.Delete().Where(giftidea.HasPersonWith(person.IDEQ(id))).Exec(ctx); err != nil {
		return err
	}
	if _, err := r.client.Relationship.Delete().Where(relationship.Or(relationship.FromIDEQ(id), relationship.ToIDEQ(id))).Exec(ctx); err != nil {
		return err
	}
	return r.client.Person.DeleteOneID(id).Exec(ctx)
}

func (r *PersonRepository) FindByGoogleResourceName(ctx context.Context, rn string) (*ent.Person, error) {
	return r.client.Person.Query().Where(person.GoogleResourceName(rn)).Only(ctx)
}

func (r *PersonRepository) ListGoogleSynced(ctx context.Context) ([]*ent.Person, error) {
	return r.client.Person.Query().Where(person.GoogleResourceNameNotNil()).Order(ent.Asc(person.FieldName)).All(ctx)
}

func (r *PersonRepository) SetGoogleResourceName(ctx context.Context, id int, rn string) (*ent.Person, error) {
	return r.client.Person.UpdateOneID(id).SetGoogleResourceName(rn).SetUpdatedAt(time.Now()).Save(ctx)
}

func (r *PersonRepository) ClearGoogleResourceName(ctx context.Context, id int) (*ent.Person, error) {
	return r.client.Person.UpdateOneID(id).ClearGoogleResourceName().SetUpdatedAt(time.Now()).Save(ctx)
}

func (r *PersonRepository) UpdateGoogleFields(ctx context.Context, id int, name, notes string) (*ent.Person, error) {
	return r.client.Person.UpdateOneID(id).SetName(name).SetNotes(notes).SetUpdatedAt(time.Now()).Save(ctx)
}

// nillableStrOrNil converts an empty string to nil so ent can clear a
// nullable column when the value is intentionally absent.
func nillableStrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
