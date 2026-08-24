package auditlog

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/repository"
)

type Repo interface {
	Append(ctx context.Context, e *ent.AuditEntry) (*ent.AuditEntry, error)
}

type Recorder struct {
	repo Repo
}

func New(repo Repo) *Recorder {
	return &Recorder{repo: repo}
}

func NewFromClient(client *ent.Client) *Recorder {
	return New(repository.NewAuditEntryRepository(client))
}

type contextKey string

func actorFromContext(ctx context.Context) string {
	// Primary: user stored under the auditlog typed key (see ContextWithUser).
	v := ctx.Value(contextKey("user"))
	if u, ok := v.(*ent.User); ok && u != nil {
		return u.Username
	}
	// Fallback: web.Auth also stores the user under a plain string key so
	// packages can read it without importing web's private key type.
	if u := findUserInContext(ctx); u != nil {
		return u.Username
	}
	return "system"
}

func findUserInContext(ctx context.Context) *ent.User {
	if v := ctx.Value("user"); v != nil {
		if u, ok := v.(*ent.User); ok && u != nil {
			return u
		}
	}
	return nil
}

// ContextWithUser stores user under the auditlog key for consistent extraction.
func ContextWithUser(ctx context.Context, u *ent.User) context.Context {
	return context.WithValue(ctx, contextKey("user"), u)
}

func (r *Recorder) Record(req *http.Request, action, target string) {
	if r == nil || r.repo == nil {
		return
	}
	actor := actorFromContext(req.Context())
	ip := clientIP(req)
	entry := &ent.AuditEntry{
		CreatedAt:     time.Now(),
		ActorUsername: actor,
		Action:        action,
		Target:        target,
		SourceIP:      ip,
	}
	if _, err := r.repo.Append(req.Context(), entry); err != nil {
		slog.Warn("audit log append failed", "action", action, "actor", actor, "error", err)
	}
}

func (r *Recorder) RecordWithActor(ctx context.Context, actor, ip, action, target string) {
	if r == nil || r.repo == nil {
		return
	}
	entry := &ent.AuditEntry{
		CreatedAt:     time.Now(),
		ActorUsername: actor,
		Action:        action,
		Target:        target,
		SourceIP:      ip,
	}
	if _, err := r.repo.Append(ctx, entry); err != nil {
		slog.Warn("audit log append failed", "action", action, "actor", actor, "error", err)
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func ClientIP(r *http.Request) string { return clientIP(r) }
