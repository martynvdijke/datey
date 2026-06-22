package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/session"
	"github.com/datey/datey/ent/user"
)

const (
	TokenLength   = 32
	CookieName    = "session"
	CookieMaxAge  = 60 * 60 * 24 * 30 // 30 days
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

// GenerateToken creates a cryptographically random token and returns the raw token and its SHA-256 hash.
func GenerateToken() (string, string, error) {
	b := make([]byte, TokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	raw := hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(hash[:]), nil
}

// Create creates a new session for the given user and returns the raw token.
func (s *Store) Create(ctx context.Context, userID int) (string, error) {
	raw, hash, err := GenerateToken()
	if err != nil {
		return "", err
	}

	_, err = s.client.Session.Create().
		SetTokenHash(hash).
		SetUserID(userID).
		SetExpiresAt(time.Now().Add(CookieMaxAge * time.Second)).
		SetCreatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	return raw, nil
}

// GetByToken looks up a session by its raw token. Returns the session if valid and not expired.
func (s *Store) GetByToken(ctx context.Context, rawToken string) (*ent.Session, error) {
	hash := sha256.Sum256([]byte(rawToken))
	hashStr := hex.EncodeToString(hash[:])

	sess, err := s.client.Session.Query().
		Where(session.TokenHashEQ(hashStr)).
		WithUser().
		Only(ctx)
	if err != nil {
		return nil, err
	}

	if time.Now().After(sess.ExpiresAt) {
		// Session expired — clean it up
		if err := s.client.Session.DeleteOne(sess).Exec(ctx); err != nil {
			slog.Warn("delete expired session", "error", err)
		}
		return nil, fmt.Errorf("session expired")
	}

	return sess, nil
}

// Delete removes a session by raw token.
func (s *Store) Delete(ctx context.Context, rawToken string) error {
	hash := sha256.Sum256([]byte(rawToken))
	hashStr := hex.EncodeToString(hash[:])

	_, err := s.client.Session.Delete().
		Where(session.TokenHashEQ(hashStr)).
		Exec(ctx)
	return err
}

// DeleteByUserID removes all sessions for a given user.
func (s *Store) DeleteByUserID(ctx context.Context, userID int) error {
	_, err := s.client.Session.Delete().
		Where(session.HasUserWith(user.IDEQ(userID))).
		Exec(ctx)
	return err
}

// CleanupExpired removes all expired sessions from the database.
func (s *Store) CleanupExpired(ctx context.Context) (int, error) {
	deleted, err := s.client.Session.Delete().
		Where(session.ExpiresAtLT(time.Now())).
		Exec(ctx)
	return deleted, err
}

// SetCookie sets the session cookie on the response.
func SetCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   CookieMaxAge,
	})
}

// ClearCookie clears the session cookie.
func ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// ReadCookie reads the session token from the request cookie.
func ReadCookie(r *http.Request) (string, error) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return "", err
	}
	if c.Value == "" {
		return "", fmt.Errorf("empty session cookie")
	}
	return c.Value, nil
}


