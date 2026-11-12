package repository

import (
	"context"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/giftidea"
	"github.com/datey/datey/ent/person"
)

type GiftIdeaRepository struct {
	client *ent.Client
}

func NewGiftIdeaRepository(client *ent.Client) *GiftIdeaRepository {
	return &GiftIdeaRepository{client: client}
}

func (r *GiftIdeaRepository) Create(ctx context.Context, personID int, title, notes string, priceCents *int, url string) (*ent.GiftIdea, error) {
	b := r.client.GiftIdea.Create().
		SetTitle(title).
		SetNotes(notes).
		SetURL(url).
		SetStatus(giftidea.StatusIdea).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		SetPersonID(personID)
	if priceCents != nil {
		b = b.SetPriceCents(*priceCents)
	}
	return b.Save(ctx)
}

func (r *GiftIdeaRepository) ListByPerson(ctx context.Context, personID int) ([]*ent.GiftIdea, error) {
	return r.client.GiftIdea.Query().
		Where(giftidea.HasPersonWith(person.IDEQ(personID))).
		Order(ent.Asc(giftidea.FieldCreatedAt)).
		All(ctx)
}

func (r *GiftIdeaRepository) ListByPersonFiltered(ctx context.Context, personID int, includePurchased bool) ([]*ent.GiftIdea, error) {
	q := r.client.GiftIdea.Query().
		Where(giftidea.HasPersonWith(person.IDEQ(personID)))
	if !includePurchased {
		q = q.Where(giftidea.StatusEQ(giftidea.StatusIdea))
	}
	return q.Order(ent.Asc(giftidea.FieldCreatedAt)).All(ctx)
}

// CountOpenByPerson returns how many open ("idea") gift ideas a person has.
// Purchased, given and archived ideas are closed and never counted.
func (r *GiftIdeaRepository) CountOpenByPerson(ctx context.Context, personID int) (int, error) {
	return r.client.GiftIdea.Query().
		Where(
			giftidea.StatusEQ(giftidea.StatusIdea),
			giftidea.HasPersonWith(person.IDEQ(personID)),
		).
		Count(ctx)
}

// CountOpenByPersonIDs returns the number of open ("idea") gift ideas per
// person for every given person ID in one grouped query, so callers listing
// many people never issue a query per row. Person IDs without an open idea are
// absent from the map (callers read the zero value).
func (r *GiftIdeaRepository) CountOpenByPersonIDs(ctx context.Context, personIDs []int) (map[int]int, error) {
	out := make(map[int]int, len(personIDs))
	if len(personIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		PersonID int `json:"person_gift_ideas"`
		Count    int `json:"count"`
	}
	err := r.client.GiftIdea.Query().
		Where(
			giftidea.StatusEQ(giftidea.StatusIdea),
			giftidea.HasPersonWith(person.IDIn(personIDs...)),
		).
		GroupBy(giftidea.PersonColumn).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.PersonID] = row.Count
	}
	return out, nil
}

// ListAll returns every gift idea with its person eager-loaded, for
// library-wide aggregates such as the stats page. Two queries in total (ideas
// + persons), never one per idea.
func (r *GiftIdeaRepository) ListAll(ctx context.Context) ([]*ent.GiftIdea, error) {
	return r.client.GiftIdea.Query().
		WithPerson().
		Order(ent.Asc(giftidea.FieldID)).
		All(ctx)
}

// applyStatusTimestamps records the lifecycle timestamp that belongs to a
// status change. A timestamp is only written when the row does not carry one
// yet, so repeating a transition (or re-saving the same status while editing)
// never rewrites history and an earlier PurchasedAt survives a later "given".
// Reverting to "idea" clears the lifecycle entirely, because the row is an
// idea again and any later purchase is a fresh spend event.
func applyStatusTimestamps(upd *ent.GiftIdeaUpdateOne, current *ent.GiftIdea, status giftidea.Status, now time.Time) *ent.GiftIdeaUpdateOne {
	switch status {
	case giftidea.StatusPurchased:
		if current.PurchasedAt == nil {
			upd = upd.SetPurchasedAt(now)
		}
	case giftidea.StatusGiven:
		if current.GivenAt == nil {
			upd = upd.SetGivenAt(now)
		}
	case giftidea.StatusArchived:
		if current.ArchivedAt == nil {
			upd = upd.SetArchivedAt(now)
		}
	case giftidea.StatusIdea:
		upd = upd.ClearPurchasedAt().ClearGivenAt().ClearArchivedAt()
	}
	return upd
}

// SetStatus is the single way gift-idea status changes. It keeps the lifecycle
// timestamps (PurchasedAt/GivenAt/ArchivedAt) consistent with the new status so
// every caller — web form, JSON API, bulk import — produces the same history.
func (r *GiftIdeaRepository) SetStatus(ctx context.Context, id int, status giftidea.Status) (*ent.GiftIdea, error) {
	current, err := r.client.GiftIdea.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	upd := r.client.GiftIdea.UpdateOneID(id).
		SetStatus(status).
		SetUpdatedAt(now)
	upd = applyStatusTimestamps(upd, current, status, now)
	return upd.Save(ctx)
}

// UpdateStatus is kept as an alias of SetStatus for existing callers; new code
// should use SetStatus.
func (r *GiftIdeaRepository) UpdateStatus(ctx context.Context, id int, status giftidea.Status) (*ent.GiftIdea, error) {
	return r.SetStatus(ctx, id, status)
}

func (r *GiftIdeaRepository) Delete(ctx context.Context, id int) error {
	return r.client.GiftIdea.DeleteOneID(id).Exec(ctx)
}

func (r *GiftIdeaRepository) Get(ctx context.Context, id int) (*ent.GiftIdea, error) {
	return r.client.GiftIdea.Get(ctx, id)
}

func (r *GiftIdeaRepository) Update(ctx context.Context, id int, title, notes string, priceCents *int, url string, status giftidea.Status) (*ent.GiftIdea, error) {
	current, err := r.client.GiftIdea.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	upd := r.client.GiftIdea.UpdateOneID(id).
		SetTitle(title).
		SetNotes(notes).
		SetURL(url).
		SetStatus(status).
		SetUpdatedAt(now)
	upd = applyStatusTimestamps(upd, current, status, now)
	if priceCents != nil {
		upd = upd.SetPriceCents(*priceCents)
	} else {
		upd = upd.ClearPriceCents()
	}
	return upd.Save(ctx)
}

// GiftMonthSpend is one bucket of the gift-spend-by-month breakdown. Month is
// the first day of that month in the instance timezone; Undated is the explicit
// bucket for priced purchases that carry no lifecycle timestamp at all.
type GiftMonthSpend struct {
	Month   time.Time
	Undated bool
	Cents   int
	Count   int
}

// MonthlySpend groups spend over the last `months` months (the current month
// included), oldest first, from a list already loaded by ListAll. GivenAt is
// the spend date when present; otherwise the purchase date is used. Rows
// outside the window are omitted, and rows with no timestamp fall into a
// single trailing undated bucket instead of being silently dropped. Pure Go
// grouping over one query result, so the stats page stays at one ListAll call.
func MonthlySpend(ideas []*ent.GiftIdea, now time.Time, months int, loc *time.Location) []GiftMonthSpend {
	if months <= 0 {
		return nil
	}
	if loc == nil {
		loc = time.Local
	}
	now = now.In(loc)
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	start := thisMonth.AddDate(0, -(months - 1), 0)

	byMonth := make(map[string]*GiftMonthSpend, months)
	var undated GiftMonthSpend
	undated.Undated = true
	for _, g := range ideas {
		// An idea is not spend; it may later become one.
		if g.Status == giftidea.StatusIdea || g.PriceCents == nil {
			continue
		}
		cents := *g.PriceCents
		if cents < 0 {
			cents = 0
		}
		var at *time.Time
		if g.GivenAt != nil {
			at = g.GivenAt
		} else {
			at = g.PurchasedAt
		}
		if at == nil {
			undated.Cents += cents
			undated.Count++
			continue
		}
		t := at.In(loc)
		month := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
		if month.Before(start) || month.After(thisMonth) {
			continue
		}
		key := month.Format("2006-01")
		bucket, ok := byMonth[key]
		if !ok {
			bucket = &GiftMonthSpend{Month: month}
			byMonth[key] = bucket
		}
		bucket.Cents += cents
		bucket.Count++
	}

	out := make([]GiftMonthSpend, 0, len(byMonth)+1)
	for i := 0; i < months; i++ {
		month := start.AddDate(0, i, 0)
		if bucket, ok := byMonth[month.Format("2006-01")]; ok && bucket.Count > 0 {
			out = append(out, *bucket)
		}
	}
	if undated.Count > 0 {
		out = append(out, undated)
	}
	return out
}
