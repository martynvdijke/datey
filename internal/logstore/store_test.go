package logstore

import (
	"log/slog"
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	s := NewStore(100)
	if s.EnsureCapacity() != 100 {
		t.Errorf("expected capacity 100, got %d", s.EnsureCapacity())
	}
	if s.Level() != slog.LevelInfo {
		t.Errorf("expected default level info, got %v", s.Level())
	}
}

func TestAppendAndQuery(t *testing.T) {
	s := NewStore(10)
	t1 := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	s.Append(LogEntry{Timestamp: t1, Level: "info", Message: "third"})
	s.Append(LogEntry{Timestamp: t2, Level: "warn", Message: "second"})
	s.Append(LogEntry{Timestamp: t3, Level: "error", Message: "first"})

	entries, total := s.Query("", "", 0, 10)
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Should be newest first (reverse chronological)
	if entries[0].Message != "first" {
		t.Errorf("expected newest 'first' (t3), got %q", entries[0].Message)
	}
	if entries[1].Message != "second" {
		t.Errorf("expected middle 'second' (t2), got %q", entries[1].Message)
	}
	if entries[2].Message != "third" {
		t.Errorf("expected oldest 'third' (t1), got %q", entries[2].Message)
	}
}

func TestAppendOverflow(t *testing.T) {
	s := NewStore(2)
	s.Append(LogEntry{Message: "one"})
	s.Append(LogEntry{Message: "two"})
	s.Append(LogEntry{Message: "three"})

	entries, total := s.Query("", "", 0, 10)
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Message != "three" {
		t.Errorf("expected newest 'three', got %q", entries[0].Message)
	}
	if entries[1].Message != "two" {
		t.Errorf("expected 'two', got %q", entries[1].Message)
	}
}

func TestQueryFilterByLevel(t *testing.T) {
	s := NewStore(10)
	s.Append(LogEntry{Level: "debug", Message: "d"})
	s.Append(LogEntry{Level: "info", Message: "i"})
	s.Append(LogEntry{Level: "warn", Message: "w"})
	s.Append(LogEntry{Level: "error", Message: "e"})

	entries, total := s.Query("warn,error", "", 0, 10)
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Message != "e" {
		t.Errorf("expected 'e' (newest warn/error), got %q", entries[0].Message)
	}
	if entries[1].Message != "w" {
		t.Errorf("expected 'w', got %q", entries[1].Message)
	}
}

func TestQueryFilterBySource(t *testing.T) {
	s := NewStore(10)
	s.Append(LogEntry{Source: "http", Message: "req"})
	s.Append(LogEntry{Source: "database", Message: "db"})
	s.Append(LogEntry{Source: "HTTP", Message: "other"})

	entries, total := s.Query("", "http", 0, 10)
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestQueryPagination(t *testing.T) {
	s := NewStore(20)
	for i := range 10 {
		s.Append(LogEntry{Message: string(rune('A' + i))})
	}

	entries, total := s.Query("", "", 2, 3)
	if total != 10 {
		t.Errorf("expected total 10, got %d", total)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
}

func TestQueryEmpty(t *testing.T) {
	s := NewStore(10)
	entries, total := s.Query("", "", 0, 10)
	if entries != nil {
		t.Errorf("expected nil for empty store, got %v", entries)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
}

func TestQueryNoMatch(t *testing.T) {
	s := NewStore(10)
	s.Append(LogEntry{Level: "info", Message: "test"})

	entries, total := s.Query("error", "", 0, 10)
	if entries != nil {
		t.Errorf("expected nil for no match, got %v", entries)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
}

func TestInitLevelAndSetLevel(t *testing.T) {
	s := NewStore(10)

	// Default should be info
	if s.Level() != slog.LevelInfo {
		t.Errorf("expected info, got %v", s.Level())
	}

	s.InitLevel(slog.LevelDebug)
	if s.Level() != slog.LevelDebug {
		t.Errorf("expected debug after InitLevel, got %v", s.Level())
	}

	s.SetLevel(slog.LevelError)
	if s.Level() != slog.LevelError {
		t.Errorf("expected error after SetLevel, got %v", s.Level())
	}
}

func TestLevelVar(t *testing.T) {
	s := NewStore(10)
	lv := s.LevelVar()
	if lv == nil {
		t.Fatal("LevelVar returned nil")
	}
	if lv.Level() != slog.LevelInfo {
		t.Errorf("expected info level, got %v", lv.Level())
	}

	lv.Set(slog.LevelDebug)
	if s.Level() != slog.LevelDebug {
		t.Errorf("expected debug after LevelVar.Set, got %v", s.Level())
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		level slog.Level
		valid bool
	}{
		{"debug", slog.LevelDebug, true},
		{"info", slog.LevelInfo, true},
		{"warn", slog.LevelWarn, true},
		{"error", slog.LevelError, true},
		{"DEBUG", slog.LevelDebug, true},
		{"Info", slog.LevelInfo, true},
		{"invalid", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level, ok := ParseLogLevel(tt.input)
			if ok != tt.valid {
				t.Errorf("valid = %v, want %v", ok, tt.valid)
			}
			if ok && level != tt.level {
				t.Errorf("level = %v, want %v", level, tt.level)
			}
		})
	}
}

func TestLevelName(t *testing.T) {
	tests := []struct {
		level slog.Level
		name  string
	}{
		{slog.LevelDebug, "debug"},
		{slog.LevelInfo, "info"},
		{slog.LevelWarn, "warn"},
		{slog.LevelError, "error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LevelName(tt.level); got != tt.name {
				t.Errorf("LevelName(%v) = %q, want %q", tt.level, got, tt.name)
			}
		})
	}
}

func TestSourceFromAttrs(t *testing.T) {
	tests := []struct {
		name  string
		attrs []slog.Attr
		want  string
	}{
		{"source key", []slog.Attr{slog.String("source", "http")}, "http"},
		{"package key", []slog.Attr{slog.String("package", "main")}, "main"},
		{"no match", []slog.Attr{slog.String("other", "val")}, ""},
		{"empty", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SourceFromAttrs(tt.attrs); got != tt.want {
				t.Errorf("SourceFromAttrs() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAttrsToMap(t *testing.T) {
	tests := []struct {
		name  string
		attrs []slog.Attr
		want  int // expected number of entries
	}{
		{"nil", nil, 0},
		{"empty", []slog.Attr{}, 0},
		{"one", []slog.Attr{slog.String("key", "val")}, 1},
		{"multiple", []slog.Attr{slog.Int("count", 5), slog.Bool("flag", true)}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := AttrsToMap(tt.attrs)
			if len(m) != tt.want {
				t.Errorf("expected %d entries, got %d", tt.want, len(m))
			}
		})
	}
}

func TestEnsureCapacity(t *testing.T) {
	s := NewStore(50)
	if cap := s.EnsureCapacity(); cap != 50 {
		t.Errorf("expected 50, got %d", cap)
	}
}

func TestString(t *testing.T) {
	s := NewStore(100)
	str := s.String()
	if str == "" {
		t.Error("expected non-empty string")
	}
}
