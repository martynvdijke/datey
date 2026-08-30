package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/apitoken"
	"github.com/datey/datey/ent/user"
)

const (
	// APITokenLength is the number of random bytes used to generate a token secret.
	APITokenLength = 32
)

// GenerateToken creates a cryptographically random bearer token secret and
// returns the raw secret together with its SHA-256 hex hash. Only the hash
// is ever persisted; the raw secret is shown to the owner exactly once.
func GenerateToken() (string, string, error) {
	b := make([]byte, APITokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate api token: %w", err)
	}
	raw := hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(hash[:]), nil
}

// HashToken returns the SHA-256 hex hash of a raw bearer token secret.
func HashToken(raw string) string {
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}

type ApiTokenRepository struct { //nolint:staticcheck // ST1003: matches ent type ApiToken
	client *ent.Client
}

func NewApiTokenRepository(client *ent.Client) *ApiTokenRepository { //nolint:staticcheck // ST1003: matches ent type ApiToken
	return &ApiTokenRepository{client: client}
}

// Create provisions a new API token for the given user. expiresAt may be nil
// for a non-expiring token. Returns the raw secret (shown once) and the
// stored entity.
func (r *ApiTokenRepository) Create(ctx context.Context, userID int, name string, expiresAt *time.Time) (string, *ent.ApiToken, error) {
	raw, hash, err := GenerateToken()
	if err != nil {
		return "", nil, err
	}

	create := r.client.ApiToken.Create().
		SetTokenHash(hash).
		SetName(name).
		SetUserID(userID).
		SetCreatedAt(time.Now())
	if expiresAt != nil {
		create = create.SetExpiresAt(*expiresAt)
	}

	tok, err := create.Save(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("create api token: %w", err)
	}
	return raw, tok, nil
}

// GetByToken looks up an API token by its raw secret. It returns an error if
// the token does not exist, has been revoked, or has expired. The owner user
// is eagerly loaded.
func (r *ApiTokenRepository) GetByToken(ctx context.Context, raw string) (*ent.ApiToken, error) {
	tok, err := r.client.ApiToken.Query().
		Where(apitoken.TokenHashEQ(HashToken(raw))).
		WithUser().
		Only(ctx)
	if err != nil {
		return nil, err
	}
	if tok.RevokedAt != nil {
		return nil, fmt.Errorf("api token revoked")
	}
	if tok.ExpiresAt != nil && time.Now().After(*tok.ExpiresAt) {
		return nil, fmt.Errorf("api token expired")
	}
	return tok, nil
}

// ListByUser returns all API tokens owned by the given user, newest first.
func (r *ApiTokenRepository) ListByUser(ctx context.Context, userID int) ([]*ent.ApiToken, error) {
	return r.client.ApiToken.Query().
		Where(apitoken.HasUserWith(user.IDEQ(userID))).
		Order(ent.Desc(apitoken.FieldCreatedAt)).
		All(ctx)
}

// GetOwned fetches a token by ID scoped to its owner. Returns an error when
// the token does not exist or belongs to another user.
func (r *ApiTokenRepository) GetOwned(ctx context.Context, id, userID int) (*ent.ApiToken, error) {
	return r.client.ApiToken.Query().
		Where(
			apitoken.IDEQ(id),
			apitoken.HasUserWith(user.IDEQ(userID)),
		).
		Only(ctx)
}

// Revoke marks a token revoked (owner-scoped). Revoked tokens stop working
// immediately but remain listed until deleted.
func (r *ApiTokenRepository) Revoke(ctx context.Context, id, userID int) error {
	n, err := r.client.ApiToken.Update().
		Where(
			apitoken.IDEQ(id),
			apitoken.HasUserWith(user.IDEQ(userID)),
			apitoken.RevokedAtIsNil(),
		).
		SetRevokedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("revoke api token: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("api token not found")
	}
	return nil
}

// Rotate replaces a token's secret with a freshly generated one (owner-scoped),
// clears any revocation, and optionally updates expiry. Returns the new raw
// secret (shown once).
func (r *ApiTokenRepository) Rotate(ctx context.Context, id, userID int, expiresAt *time.Time) (string, error) {
	raw, hash, err := GenerateToken()
	if err != nil {
		return "", err
	}

	update := r.client.ApiToken.Update().
		Where(
			apitoken.IDEQ(id),
			apitoken.HasUserWith(user.IDEQ(userID)),
		).
		SetTokenHash(hash).
		ClearRevokedAt()
	if expiresAt != nil {
		update = update.SetExpiresAt(*expiresAt)
	} else {
		update = update.ClearExpiresAt()
	}

	n, err := update.Save(ctx)
	if err != nil {
		return "", fmt.Errorf("rotate api token: %w", err)
	}
	if n == 0 {
		return "", fmt.Errorf("api token not found")
	}
	return raw, nil
}

// UpdateLastUsed records successful use of a token. Failures are ignored by
// callers; this is best-effort telemetry.
func (r *ApiTokenRepository) UpdateLastUsed(ctx context.Context, id int) {
	_ = r.client.ApiToken.UpdateOneID(id).SetLastUsedAt(time.Now()).Exec(ctx)
}
