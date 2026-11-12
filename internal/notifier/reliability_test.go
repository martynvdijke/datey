package notifier

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// fakeStore records health writes and can be told to fail.
type fakeStore struct {
	successes []string
	failures  []fakeFailure
	err       error
}

type fakeFailure struct {
	channel string
	target  string
	message string
}

func (f *fakeStore) RecordSuccess(_ context.Context, channel, target string) error {
	f.successes = append(f.successes, channel+"|"+target)
	return f.err
}

func (f *fakeStore) RecordFailure(_ context.Context, channel, target, message string) error {
	f.failures = append(f.failures, fakeFailure{channel: channel, target: target, message: message})
	return f.err
}

// fixedPolicy returns a policy with jitter disabled and sleeps captured.
func fixedPolicy(delays *[]time.Duration, attempts int) RetryPolicy {
	return RetryPolicy{
		MaxAttempts: attempts,
		BaseDelay:   500 * time.Millisecond,
		Multiplier:  2,
		MaxDelay:    8 * time.Second,
		Jitter:      func(time.Duration) time.Duration { return 0 },
		Sleep: func(_ context.Context, d time.Duration) error {
			*delays = append(*delays, d)
			return nil
		},
	}
}

func TestRetryPolicy_Do(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name        string
		attempts    int
		fn          func(ctx context.Context, call int) error
		wantCalls   int
		wantErr     error
		wantDelays  []time.Duration
		wantAttempt int
	}{
		{
			name:     "retry then success",
			attempts: 3,
			fn: func(_ context.Context, call int) error {
				if call < 3 {
					return boom
				}
				return nil
			},
			wantCalls:   3,
			wantDelays:  []time.Duration{500 * time.Millisecond, time.Second},
			wantAttempt: 3,
		},
		{
			name:     "exhaust attempts",
			attempts: 3,
			fn: func(context.Context, int) error {
				return boom
			},
			wantCalls:   3,
			wantErr:     boom,
			wantDelays:  []time.Duration{500 * time.Millisecond, time.Second},
			wantAttempt: 3,
		},
		{
			name:     "context cancellation is never retried",
			attempts: 3,
			fn: func(context.Context, int) error {
				return fmt.Errorf("send: %w", context.Canceled)
			},
			wantCalls:   1,
			wantErr:     context.Canceled,
			wantDelays:  nil,
			wantAttempt: 1,
		},
		{
			name:     "deadline exceeded is never retried",
			attempts: 3,
			fn: func(context.Context, int) error {
				return fmt.Errorf("send: %w", context.DeadlineExceeded)
			},
			wantCalls:   1,
			wantErr:     context.DeadlineExceeded,
			wantDelays:  nil,
			wantAttempt: 1,
		},
		{
			name:     "permanent error is not retried",
			attempts: 3,
			fn: func(context.Context, int) error {
				return Permanent(boom)
			},
			wantCalls:   1,
			wantErr:     boom,
			wantDelays:  nil,
			wantAttempt: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var delays []time.Duration
			policy := fixedPolicy(&delays, tt.attempts)
			calls := 0
			attempts, err := policy.Do(context.Background(), func(ctx context.Context) error {
				calls++
				return tt.fn(ctx, calls)
			})
			if tt.wantErr == nil && err != nil {
				t.Fatalf("Do() error = %v, want nil", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Do() error = %v, want %v", err, tt.wantErr)
			}
			if calls != tt.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tt.wantCalls)
			}
			if attempts != tt.wantAttempt {
				t.Errorf("attempts = %d, want %d", attempts, tt.wantAttempt)
			}
			if len(delays) != len(tt.wantDelays) {
				t.Fatalf("delays = %v, want %v", delays, tt.wantDelays)
			}
			for i := range delays {
				if delays[i] != tt.wantDelays[i] {
					t.Errorf("delay[%d] = %v, want %v", i, delays[i], tt.wantDelays[i])
				}
			}
		})
	}
}

func TestRetryPolicy_Do_ContextAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var delays []time.Duration
	calls := 0
	attempts, err := fixedPolicy(&delays, 3).Do(ctx, func(context.Context) error {
		calls++
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if calls != 0 {
		t.Errorf("call count = %d, want 0", calls)
	}
	if attempts != 0 {
		t.Errorf("attempts = %d, want 0", attempts)
	}
}

func TestRetryPolicy_Do_SleepCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	policy := RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   time.Hour,
		Multiplier:  2,
		MaxDelay:    time.Hour,
		Jitter:      func(time.Duration) time.Duration { return 0 },
		Sleep: func(context.Context, time.Duration) error {
			cancel()
			return context.Canceled
		},
	}
	attempts, err := policy.Do(ctx, func(context.Context) error { return errors.New("boom") })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}

func TestRetryPolicy_Delay(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts: 6,
		BaseDelay:   500 * time.Millisecond,
		Multiplier:  2,
		MaxDelay:    8 * time.Second,
		Jitter:      func(time.Duration) time.Duration { return 0 },
	}
	want := []time.Duration{
		500 * time.Millisecond,
		time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		8 * time.Second, // capped
	}
	var previous time.Duration
	for i, expected := range want {
		got := policy.Delay(i + 1)
		if got != expected {
			t.Errorf("Delay(%d) = %v, want %v", i+1, got, expected)
		}
		if i > 0 && got < previous {
			t.Errorf("Delay(%d) = %v is not monotonic (previous %v)", i+1, got, previous)
		}
		previous = got
	}
}

func TestRetryPolicy_Delay_JitterIsBounded(t *testing.T) {
	policy := DefaultRetryPolicy()
	for i := 1; i <= DefaultMaxAttempts; i++ {
		withoutJitter := RetryPolicy{
			MaxAttempts: policy.MaxAttempts,
			BaseDelay:   policy.BaseDelay,
			Multiplier:  policy.Multiplier,
			MaxDelay:    policy.MaxDelay,
			Jitter:      func(time.Duration) time.Duration { return 0 },
		}.Delay(i)
		got := policy.Delay(i)
		if got < withoutJitter {
			t.Errorf("Delay(%d) = %v, want >= %v", i, got, withoutJitter)
		}
		if limit := withoutJitter + withoutJitter/10; got > limit {
			t.Errorf("Delay(%d) = %v exceeds jitter ceiling %v", i, got, limit)
		}
	}
}

func TestHealthRecorder_RecordSuccess(t *testing.T) {
	store := &fakeStore{}
	rec := NewHealthRecorder(store)
	rec.Record(context.Background(), "telegram", "chat-1", nil)
	if len(store.successes) != 1 || store.successes[0] != "telegram|chat-1" {
		t.Fatalf("successes = %v", store.successes)
	}
	if len(store.failures) != 0 {
		t.Fatalf("failures = %v, want none", store.failures)
	}
}

func TestHealthRecorder_RecordFailure(t *testing.T) {
	store := &fakeStore{}
	rec := NewHealthRecorder(store)
	rec.Record(context.Background(), "email", "a@example.com", errors.New("smtp down"))
	if len(store.failures) != 1 {
		t.Fatalf("failures = %v", store.failures)
	}
	got := store.failures[0]
	if got.channel != "email" || got.target != "a@example.com" || got.message != "smtp down" {
		t.Errorf("failure = %+v", got)
	}
}

func TestHealthRecorder_TruncatesLongErrors(t *testing.T) {
	store := &fakeStore{}
	rec := NewHealthRecorder(store)
	rec.Record(context.Background(), "webhook", "https://x", errors.New(strings.Repeat("ä", 900)))
	if len(store.failures) != 1 {
		t.Fatalf("failures = %v", store.failures)
	}
	msg := store.failures[0].message
	if got := len([]rune(msg)); got != defaultMaxErrorSize {
		t.Errorf("stored rune length = %d, want %d", got, defaultMaxErrorSize)
	}
	if !strings.HasSuffix(msg, "…") {
		t.Errorf("stored message does not end in ellipsis")
	}
	if strings.ContainsRune(msg, '\uFFFD') {
		t.Errorf("stored message contains a broken character")
	}
}

func TestHealthRecorder_CustomBudget(t *testing.T) {
	store := &fakeStore{}
	rec := &HealthRecorder{Store: store, MaxErrorRunes: 10}
	rec.Record(context.Background(), "ntfy", "topic", errors.New("0123456789abcdef"))
	if got := store.failures[0].message; got != "012345678…" {
		t.Errorf("message = %q", got)
	}
}

func TestHealthRecorder_StoreFailureDoesNotPanic(t *testing.T) {
	store := &fakeStore{err: errors.New("db down")}
	rec := NewHealthRecorder(store)
	rec.Record(context.Background(), "discord", "", errors.New("boom"))
	rec.Record(context.Background(), "discord", "", nil)
}

func TestHealthRecorder_NilStoreIsSafe(t *testing.T) {
	rec := NewHealthRecorder(nil)
	rec.Record(context.Background(), "slack", "", errors.New("boom"))
	var nilRec *HealthRecorder
	nilRec.Record(context.Background(), "slack", "", nil)
}

func TestTruncateError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		limit int
		want  string
	}{
		{name: "short stays untouched", input: "boom", limit: 500, want: "boom"},
		{name: "exact boundary untouched", input: "abcde", limit: 5, want: "abcde"},
		{name: "truncated with ellipsis", input: "abcdef", limit: 5, want: "abcd…"},
		{name: "single rune budget", input: "abcdef", limit: 1, want: "…"},
		{name: "zero limit uses default", input: "boom", limit: 0, want: "boom"},
		{name: "multibyte not split", input: "äää", limit: 2, want: "ä…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TruncateError(tt.input, tt.limit); got != tt.want {
				t.Errorf("TruncateError(%q, %d) = %q, want %q", tt.input, tt.limit, got, tt.want)
			}
		})
	}
}

// flakyNotifier fails the given number of times before succeeding.
type flakyNotifier struct {
	name     string
	failures int
	calls    int
}

func (f *flakyNotifier) Name() string       { return f.name }
func (f *flakyNotifier) IsConfigured() bool { return true }
func (f *flakyNotifier) Send(ctx context.Context, title, message string) error {
	return f.SendTo(ctx, title, message, "")
}
func (f *flakyNotifier) SendTo(context.Context, string, string, string) error {
	f.calls++
	if f.calls <= f.failures {
		return errors.New("temporary outage")
	}
	return nil
}

func TestSender_SendTo_RetriesThenRecordsSuccess(t *testing.T) {
	var delays []time.Duration
	store := &fakeStore{}
	n := &flakyNotifier{name: "telegram", failures: 2}
	sender := &Sender{Policy: fixedPolicy(&delays, 3), Health: NewHealthRecorder(store)}
	if err := sender.SendTo(context.Background(), n, "t", "m", "chat-9"); err != nil {
		t.Fatalf("SendTo() error = %v", err)
	}
	if n.calls != 3 {
		t.Errorf("calls = %d, want 3", n.calls)
	}
	if len(delays) != 2 {
		t.Errorf("delays = %v, want 2 entries", delays)
	}
	if len(store.successes) != 1 || store.successes[0] != "telegram|chat-9" {
		t.Errorf("health successes = %v", store.successes)
	}
}

func TestSender_SendTo_ExhaustedRecordsFailure(t *testing.T) {
	var delays []time.Duration
	store := &fakeStore{}
	n := &flakyNotifier{name: "email", failures: 10}
	sender := &Sender{Policy: fixedPolicy(&delays, 2), Health: NewHealthRecorder(store)}
	err := sender.SendTo(context.Background(), n, "t", "m", "a@example.com")
	if err == nil {
		t.Fatal("expected error")
	}
	if n.calls != 2 {
		t.Errorf("calls = %d, want 2", n.calls)
	}
	if len(store.failures) != 1 || store.failures[0].message != "temporary outage" {
		t.Errorf("health failures = %+v", store.failures)
	}
}

func TestSender_SendTo_HealthFailureDoesNotMaskSend(t *testing.T) {
	var delays []time.Duration
	store := &fakeStore{err: errors.New("db down")}
	n := &flakyNotifier{name: "discord", failures: 0}
	sender := &Sender{Policy: fixedPolicy(&delays, 1), Health: NewHealthRecorder(store)}
	if err := sender.SendTo(context.Background(), n, "t", "m", ""); err != nil {
		t.Fatalf("SendTo() error = %v, want nil", err)
	}
}

func TestSender_NilHealthIsSafe(t *testing.T) {
	var delays []time.Duration
	sender := &Sender{Policy: fixedPolicy(&delays, 1)}
	n := &flakyNotifier{name: "ntfy"}
	if err := sender.Send(context.Background(), n, "t", "m"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}
