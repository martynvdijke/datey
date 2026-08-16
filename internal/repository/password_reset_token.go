package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/passwordresettoken"
	"github.com/datey/datey/ent/user"
)

// PasswordResetTokenRepository manages one-time password reset tokens.
//
// Only the SHA-256 hash of a token is stored, mirroring the session store:
// a database leak never exposes usable tokens, and a reset link can only be
// used once before it is marked used.
type PasswordResetTokenRepository struct {
	client *ent.Client
}

func NewPasswordResetTokenRepository(client *ent.Client) *PasswordResetTokenRepository {
	return &PasswordResetTokenRepository{client: client}
}

// generateResetToken creates a cryptographically random raw token and its
// SHA-256 hash (hex-encoded).
func generateResetToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate reset token: %w", err)
	}
	raw = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(sum[:]), nil
}

// Create generates a fresh reset token for the given user that expires after
// ttl. Any earlier tokens for that user (used or expired) are deleted first so
// that only the newest reset link stays valid. It returns the raw token (sent
// to the user) and its expiry.
func (r *PasswordResetTokenRepository) Create(ctx context.Context, userID int, ttl time.Duration) (string, time.Time, error) {
	// Invalidate any previously issued tokens for this user.
	if _, err := r.client.PasswordResetToken.Delete().
		Where(passwordresettoken.HasUserWith(user.IDEQ(userID))).
		Exec(ctx); err != nil {
		return "", time.Time{}, fmt.Errorf("delete prior reset tokens: %w", err)
	}

	raw, hash, err := generateResetToken()
	if err != nil {
		return "", time.Time{}, err
	}

	expiresAt := time.Now().Add(ttl)
	if _, err := r.client.PasswordResetToken.Create().
		SetTokenHash(hash).
		SetExpiresAt(expiresAt).
		SetUsed(false).
		SetCreatedAt(time.Now()).
		SetUserID(userID).
		Save(ctx); err != nil {
		return "", time.Time{}, fmt.Errorf("create reset token: %w", err)
	}

	return raw, expiresAt, nil
}

// GetByRawToken resolves a raw token to its database record. It only returns
// tokens that are unused and not yet expired; expired or used tokens are
// deleted and reported as not found.
func (r *PasswordResetTokenRepository) GetByRawToken(ctx context.Context, raw string) (*ent.PasswordResetToken, error) {
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])

	tok, err := r.client.PasswordResetToken.Query().
		Where(passwordresettoken.TokenHashEQ(hash)).
		WithUser().
		Only(ctx)
	if err != nil {
		return nil, err
	}

	if tok.Used {
		// One-time token already consumed — discard it.
		_ = r.client.PasswordResetToken.DeleteOne(tok).Exec(ctx)
		return nil, fmt.Errorf("reset token already used")
	}
	if time.Now().After(tok.ExpiresAt) {
		// Expired — clean up.
		_ = r.client.PasswordResetToken.DeleteOne(tok).Exec(ctx)
		return nil, fmt.Errorf("reset token expired")
	}

	return tok, nil
}

// MarkUsed consumes a token so it can never be used again.
func (r *PasswordResetTokenRepository) MarkUsed(ctx context.Context, id int) error {
	return r.client.PasswordResetToken.UpdateOneID(id).SetUsed(true).Exec(ctx)
}
