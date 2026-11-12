package notifier

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"time"
)

// Default retry tuning. Three attempts with a 500ms base delay, doubling up to
// an 8s ceiling, keep a transient outage recoverable without letting a broken
// channel stall the scheduler pass for long.
const (
	DefaultMaxAttempts  = 3
	DefaultBaseDelay    = 500 * time.Millisecond
	DefaultMultiplier   = 2.0
	DefaultMaxDelay     = 8 * time.Second
	defaultMaxErrorSize = 500
)

// PermanentError marks an error that retrying cannot fix (validation, bad
// credentials, malformed target). The retry loop stops immediately on it.
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// Permanent wraps err so the retry loop treats it as non-retryable. A nil err
// stays nil, which keeps call sites free of nil checks.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &PermanentError{Err: err}
}

// IsRetryable reports whether another attempt could plausibly succeed.
// Cancellation and deadline errors end the attempt chain: the caller asked to
// stop, so sending again would ignore its intent.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var permanent *PermanentError
	if errors.As(err, &permanent) {
		return false
	}
	return true
}

// RetryPolicy describes bounded exponential backoff. Every field is
// injectable so tests run instantly and deterministically.
type RetryPolicy struct {
	// MaxAttempts includes the first try. Values < 1 fall back to the default.
	MaxAttempts int
	// BaseDelay is the delay before the second attempt.
	BaseDelay time.Duration
	// Multiplier grows the delay after every failed attempt.
	Multiplier float64
	// MaxDelay caps the pre-jitter delay. Zero means DefaultMaxDelay.
	MaxDelay time.Duration
	// Jitter adds a small positive offset to a computed delay. Nil means the
	// package default (up to 10% of the delay); return 0 to disable it.
	Jitter func(time.Duration) time.Duration
	// Sleep waits for d or until ctx is done. Nil means SleepContext.
	Sleep func(ctx context.Context, d time.Duration) error
}

// DefaultRetryPolicy returns the production tuning described above.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: DefaultMaxAttempts,
		BaseDelay:   DefaultBaseDelay,
		Multiplier:  DefaultMultiplier,
		MaxDelay:    DefaultMaxDelay,
	}
}

// normalized fills zero values with defaults so a partially specified policy
// still behaves predictably.
func (p RetryPolicy) normalized() RetryPolicy {
	if p.MaxAttempts < 1 {
		p.MaxAttempts = DefaultMaxAttempts
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = DefaultBaseDelay
	}
	if p.Multiplier < 1 {
		p.Multiplier = DefaultMultiplier
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = DefaultMaxDelay
	}
	if p.Jitter == nil {
		p.Jitter = defaultJitter
	}
	if p.Sleep == nil {
		p.Sleep = SleepContext
	}
	return p
}

// defaultJitter spreads retries by up to 10% so several channels failing at
// once do not hammer an upstream in lockstep.
func defaultJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	maxJitter := int64(d) / 10
	if maxJitter <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(maxJitter + 1))
}

// Delay returns the backoff before retry attempt (1-based: attempt 1 is the
// first retry, i.e. the delay after the initial send failed).
func (p RetryPolicy) Delay(attempt int) time.Duration {
	p = p.normalized()
	if attempt < 1 {
		attempt = 1
	}
	raw := float64(p.BaseDelay) * math.Pow(p.Multiplier, float64(attempt-1))
	if capDelay := float64(p.MaxDelay); capDelay > 0 && raw > capDelay {
		raw = capDelay
	}
	d := time.Duration(raw)
	if jitter := p.Jitter(d); jitter > 0 {
		d += jitter
	}
	return d
}

// Do runs fn until it succeeds, the attempt budget is exhausted, the error is
// not retryable, or ctx is done. It returns how many attempts were made.
//
// Duplicate-delivery note: a send is retried only when it returned an error.
// A channel that accepted the payload but failed on the response path (for
// example an HTTP timeout after the request was written) can therefore be
// delivered twice. That is a deliberate trade: at-least-once delivery, because
// a missed birthday reminder is worse than a duplicate one. Attempts are
// capped at three, and the daily notification log still suppresses repeats of
// the same reminder on later scheduler passes.
func (p RetryPolicy) Do(ctx context.Context, fn func(context.Context) error) (int, error) {
	p = p.normalized()
	var lastErr error
	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return attempt - 1, err
		}
		lastErr = fn(ctx)
		if lastErr == nil {
			return attempt, nil
		}
		if !IsRetryable(lastErr) {
			return attempt, lastErr
		}
		if attempt == p.MaxAttempts {
			break
		}
		if err := p.Sleep(ctx, p.Delay(attempt)); err != nil {
			return attempt, err
		}
	}
	return p.MaxAttempts, lastErr
}

// SleepContext waits for d or until ctx is done, whichever comes first.
func SleepContext(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// HealthStore is the persistence surface the recorder needs. The ent-backed
// implementation lives in internal/repository; tests use a fake.
type HealthStore interface {
	RecordSuccess(ctx context.Context, channel, target string) error
	RecordFailure(ctx context.Context, channel, target, message string) error
}

// HealthRecorder writes the outcome of a delivery attempt to the health store
// and never lets a health-write failure affect the send result.
type HealthRecorder struct {
	Store HealthStore
	// MaxErrorRunes bounds the stored error text. Zero means 500.
	MaxErrorRunes int
	// Log receives health-write failures. Nil means slog.Default().
	Log *slog.Logger
}

func NewHealthRecorder(store HealthStore) *HealthRecorder {
	return &HealthRecorder{Store: store}
}

// Record stores the attempt outcome. A nil err is a success.
func (h *HealthRecorder) Record(ctx context.Context, channel, target string, sendErr error) {
	if h == nil || h.Store == nil {
		return
	}
	log := h.Log
	if log == nil {
		log = slog.Default()
	}
	var err error
	if sendErr == nil {
		err = h.Store.RecordSuccess(ctx, channel, target)
	} else {
		err = h.Store.RecordFailure(ctx, channel, target, TruncateError(sendErr.Error(), h.MaxErrorRunes))
	}
	if err != nil {
		log.Error("channel health: record failed",
			"source", "notifier", "channel", channel, "error", err)
	}
}

// TruncateError shortens s to at most maxRunes runes (never splitting a
// multi-byte character), keeping the tail ellipsis inside that budget.
func TruncateError(s string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = defaultMaxErrorSize
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes == 1 {
		return "…"
	}
	return string(runes[:maxRunes-1]) + "…"
}

// Sender wraps a Notifier call with retry and health recording. The Notifier
// interface itself is unchanged, so callers keep working with the registry.
type Sender struct {
	Policy RetryPolicy
	Health *HealthRecorder
	Log    *slog.Logger
}

// NewSender builds a Sender with the production retry policy. health may be
// nil to disable health recording.
func NewSender(health *HealthRecorder) *Sender {
	return &Sender{Policy: DefaultRetryPolicy(), Health: health}
}

// SendTo delivers one message to n, retrying transient failures and persisting
// the final outcome. It returns the error of the final attempt.
func (s *Sender) SendTo(ctx context.Context, n Notifier, title, message, target string) error {
	return s.SendToActions(ctx, n, title, message, target, ActionLinks{})
}

// SendToActions is SendTo with optional native action links. Channels that do
// not implement ActionNotifier, or where actions is empty, take the plain
// SendTo path, so retry and health behaviour stay identical for every channel.
func (s *Sender) SendToActions(ctx context.Context, n Notifier, title, message, target string, actions ActionLinks) error {
	if n == nil {
		return fmt.Errorf("notifier is nil")
	}
	attempts, err := s.Policy.Do(ctx, func(ctx context.Context) error {
		if an, ok := n.(ActionNotifier); ok && !actions.Empty() {
			return an.SendToActions(ctx, title, message, target, actions)
		}
		return n.SendTo(ctx, title, message, target)
	})
	s.Health.Record(ctx, n.Name(), target, err)

	log := s.Log
	if log == nil {
		log = slog.Default()
	}
	if err != nil {
		log.Error("notification failed",
			"source", "notifier", "channel", n.Name(), "attempts", attempts, "error", err)
	} else if attempts > 1 {
		log.Info("notification delivered after retry",
			"source", "notifier", "channel", n.Name(), "attempts", attempts)
	}
	return err
}

// Send is SendTo without a per-user target (the channel's global destination).
func (s *Sender) Send(ctx context.Context, n Notifier, title, message string) error {
	return s.SendTo(ctx, n, title, message, "")
}
