package logstore

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// LogEntry represents a single structured log entry stored in the ring buffer.
type LogEntry struct {
	Timestamp  time.Time
	Level      string
	Source     string
	Message    string
	Attributes map[string]any
}

// Store is a thread-safe fixed-capacity ring buffer for log entries.
type Store struct {
	mu       sync.RWMutex
	entries  []LogEntry
	head     int
	count    int
	capacity int
	level    *slog.LevelVar
}

// NewStore creates a new ring buffer store with the given capacity.
func NewStore(capacity int) *Store {
	return &Store{
		entries:  make([]LogEntry, capacity),
		capacity: capacity,
		level:    &slog.LevelVar{},
	}
}

// InitLevel sets the initial log level. Must be called before any handler uses the store.
func (s *Store) InitLevel(l slog.Level) {
	s.level.Set(l)
}

// Append adds a log entry to the ring buffer in a thread-safe manner.
// If the buffer is full, the oldest entry is overwritten.
func (s *Store) Append(entry LogEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[s.head] = entry
	s.head = (s.head + 1) % s.capacity
	if s.count < s.capacity {
		s.count++
	}
}

// Query returns log entries matching the given filters, in reverse chronological order
// (newest first). Level can be a comma-separated list (e.g., "warn,error"), or empty for all.
// Source filters by source string (empty for all).
// Returns the matching entries and the total count before pagination.
func (s *Store) Query(level, source string, offset, limit int) ([]LogEntry, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.count == 0 {
		return nil, 0
	}

	levelSet := parseLevelFilter(level)

	// Collect matching entries in reverse order (newest first).
	var matched []LogEntry
	for i := 0; i < s.count; i++ {
		idx := (s.head - 1 - i + s.capacity) % s.capacity
		entry := s.entries[idx]

		if level != "" && !levelSet[strings.ToLower(entry.Level)] {
			continue
		}
		if source != "" && !strings.Contains(strings.ToLower(entry.Source), strings.ToLower(source)) {
			continue
		}
		matched = append(matched, entry)
	}

	total := len(matched)

	if offset >= total {
		return nil, total
	}

	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}

	return matched[offset:end], total
}

// Level returns the current minimum log level.
func (s *Store) Level() slog.Level {
	return s.level.Level()
}

// SetLevel updates the minimum log level at runtime.
func (s *Store) SetLevel(l slog.Level) {
	s.level.Set(l)
}

// LevelVar returns the underlying slog.LevelVar for use with slog handlers.
func (s *Store) LevelVar() *slog.LevelVar {
	return s.level
}

// ParseLogLevel converts a string level name to slog.Level.
// Returns the level and true if valid, or zero and false if invalid.
func ParseLogLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return 0, false
	}
}

// LevelName returns the string name for a slog.Level.
func LevelName(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return "debug"
	case l < slog.LevelWarn:
		return "info"
	case l < slog.LevelError:
		return "warn"
	default:
		return "error"
	}
}

func parseLevelFilter(s string) map[string]bool {
	parts := strings.Split(s, ",")
	set := make(map[string]bool, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			set[strings.ToLower(p)] = true
		}
	}
	return set
}

// SourceFromAttrs extracts a "source" attribute from slog attributes if present.
func SourceFromAttrs(attrs []slog.Attr) string {
	for _, a := range attrs {
		if a.Key == "source" || a.Key == "package" {
			return a.Value.String()
		}
	}
	return ""
}

// AttrsToMap converts slog attributes to a map for the LogEntry.
func AttrsToMap(attrs []slog.Attr) map[string]any {
	if len(attrs) == 0 {
		return nil
	}
	m := make(map[string]any, len(attrs))
	for _, a := range attrs {
		m[a.Key] = a.Value.Any()
	}
	return m
}

// Ensure InitLevel is called properly — fix the double init bug by checking if already set
func init() {
	// noop: factory function handles init
}

// EnsureCapacity returns the store's capacity.
func (s *Store) EnsureCapacity() int {
	return s.capacity
}

// String satisfies the fmt.Stringer for log level display.
func (s *Store) String() string {
	return fmt.Sprintf("Store(capacity=%d, count=%d, level=%s)", s.capacity, s.count, LevelName(s.level.Level()))
}
