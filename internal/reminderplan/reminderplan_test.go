package reminderplan

import (
	"strings"
	"testing"
	"time"
)

func TestParseStages(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []int
		wantErr bool
	}{
		{"empty", "", nil, false},
		{"blank", "   ", nil, false},
		{"single", "7", []int{7}, false},
		{"comma", "7,1", []int{1, 7}, false},
		{"comma space", "30, 7, 1", []int{1, 7, 30}, false},
		{"space separated", "30 7 1", []int{1, 7, 30}, false},
		{"duplicates", "7,7,1,1", []int{1, 7}, false},
		{"lower bound", "1", []int{1}, false},
		{"upper bound", "365", []int{365}, false},
		{"zero", "0", nil, true},
		{"negative", "-1", nil, true},
		{"too large", "366", nil, true},
		{"non numeric", "7,abc", nil, true},
		{"float", "1.5", nil, true},
		{"eleven stages", "1,2,3,4,5,6,7,8,9,10,11", nil, true},
		{"ten stages deduped", "1,1,2,3,4,5,6,7,8,9,10", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseStages(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseStages(%q) = %v, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseStages(%q) unexpected error: %v", tc.in, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("ParseStages(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("ParseStages(%q) = %v, want %v", tc.in, got, tc.want)
				}
			}
		})
	}
}

func TestNormalizeStages(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		want []int
	}{
		{"nil", nil, []int{}},
		{"empty", []int{}, []int{}},
		{"dedupe and sort", []int{7, 1, 7}, []int{1, 7}},
		{"drops invalid", []int{0, -5, 366, 3}, []int{3}},
		{"keeps bounds", []int{365, 1}, []int{1, 365}},
		{"caps at ten", []int{20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9}, []int{9, 10, 11, 12, 13, 14, 15, 16, 17, 18}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeStages(tc.in)
			if got == nil {
				t.Fatal("NormalizeStages returned nil")
			}
			if len(got) != len(tc.want) {
				t.Fatalf("NormalizeStages(%v) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("NormalizeStages(%v) = %v, want %v", tc.in, got, tc.want)
				}
			}
		})
	}
}

func TestEffectiveStages(t *testing.T) {
	global := []int{1, 7}
	person := []int{3}
	event := []int{14, 30}

	tests := []struct {
		name                  string
		global, person, event []int
		want                  []int
	}{
		{"event wins", global, person, event, event},
		{"person when no event", global, person, nil, person},
		{"global when no overrides", global, nil, nil, global},
		{"empty everything", nil, nil, nil, []int{}},
		{"empty event list falls through", global, []int{}, []int{}, global},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EffectiveStages(tc.global, tc.person, tc.event)
			if len(got) != len(tc.want) {
				t.Fatalf("EffectiveStages = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("EffectiveStages = %v, want %v", got, tc.want)
				}
			}
		})
	}

	t.Run("returns a copy", func(t *testing.T) {
		in := []int{1, 7}
		got := EffectiveStages(in, nil, nil)
		got[0] = 99
		if in[0] != 1 {
			t.Fatalf("EffectiveStages aliased its argument: %v", in)
		}
	})
}

func TestStage(t *testing.T) {
	now := time.Date(2026, time.November, 11, 23, 30, 0, 0, time.UTC)
	tests := []struct {
		name     string
		occasion time.Time
		stages   []int
		wantN    int
		wantOK   bool
	}{
		{"stage today", time.Date(2026, time.November, 18, 0, 5, 0, 0, time.UTC), []int{1, 7}, 7, true},
		{"tomorrow stage", time.Date(2026, time.November, 12, 8, 0, 0, 0, time.UTC), []int{1, 7}, 1, true},
		{"not a stage", time.Date(2026, time.November, 15, 0, 0, 0, 0, time.UTC), []int{1, 7}, 0, false},
		{"day of is not a stage", time.Date(2026, time.November, 11, 23, 59, 0, 0, time.UTC), []int{1, 7}, 0, false},
		{"yesterday", time.Date(2026, time.November, 10, 0, 0, 0, 0, time.UTC), []int{1, 7}, 0, false},
		{"empty stages", time.Date(2026, time.November, 18, 0, 0, 0, 0, time.UTC), nil, 0, false},
		{"boundary 365", time.Date(2027, time.November, 11, 0, 0, 0, 0, time.UTC), []int{365}, 365, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotN, gotOK := Stage(now, tc.occasion, tc.stages)
			if gotN != tc.wantN || gotOK != tc.wantOK {
				t.Fatalf("Stage = (%d, %v), want (%d, %v)", gotN, gotOK, tc.wantN, tc.wantOK)
			}
		})
	}
}

func TestDigestKey(t *testing.T) {
	if got, want := DigestKey("email", "2026-11-11"), "email-2026-11-11-digest"; got != want {
		t.Fatalf("DigestKey = %q, want %q", got, want)
	}
}

func TestComposeDigest(t *testing.T) {
	if got := ComposeDigest(nil); got != "" {
		t.Fatalf("ComposeDigest(nil) = %q, want empty", got)
	}
	if got := ComposeDigest([]DigestEntry{}); got != "" {
		t.Fatalf("ComposeDigest(empty) = %q, want empty", got)
	}

	one := ComposeDigest([]DigestEntry{{Title: "Alice - birthday", Message: "November 18 (7 days away)"}})
	if !strings.HasPrefix(one, DigestHeader+"\n\n") {
		t.Fatalf("single digest missing header: %q", one)
	}
	if !strings.Contains(one, "Alice - birthday: November 18 (7 days away)") {
		t.Fatalf("single digest missing entry: %q", one)
	}
	if strings.HasSuffix(one, "\n") {
		t.Fatalf("digest has trailing newline: %q", one)
	}

	many := ComposeDigest([]DigestEntry{
		{Title: "Alice - birthday", Message: "November 18 (7 days away)"},
		{Title: "Bob - anniversary", Message: "tomorrow"},
		{Title: "Only a title"},
		{Message: "Only a message"},
	})
	if strings.Count(many, "\n\n") != 4 {
		t.Fatalf("expected blank-line separated entries, got %q", many)
	}
	for _, want := range []string{
		"Alice - birthday: November 18 (7 days away)",
		"Bob - anniversary: tomorrow",
		"Only a title",
		"Only a message",
	} {
		if !strings.Contains(many, want) {
			t.Fatalf("digest missing %q: %q", want, many)
		}
	}
	if strings.HasSuffix(many, "\n") {
		t.Fatalf("digest has trailing newline: %q", many)
	}
}
