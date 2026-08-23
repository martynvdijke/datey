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

func (r *GiftIdeaRepository) UpdateStatus(ctx context.Context, id int, status giftidea.Status) (*ent.GiftIdea, error) {
	return r.client.GiftIdea.UpdateOneID(id).
		SetStatus(status).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

func (r *GiftIdeaRepository) Delete(ctx context.Context, id int) error {
	return r.client.GiftIdea.DeleteOneID(id).Exec(ctx)
}

func (r *GiftIdeaRepository) Get(ctx context.Context, id int) (*ent.GiftIdea, error) {
	return r.client.GiftIdea.Get(ctx, id)
}
