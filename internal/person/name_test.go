package person

import "testing"

func TestDisplayName(t *testing.T) {
	tests := []struct {
		name                string
		first, middle, last string
		fallback            string
		want                string
	}{
		{name: "all parts", first: "Jane", middle: "Alice", last: "Doe", fallback: "ignored", want: "Jane Alice Doe"},
		{name: "empty middle omitted", first: "John", middle: "", last: "Doe", fallback: "ignored", want: "John Doe"},
		{name: "first only", first: "Ada", middle: "", last: "", fallback: "ignored", want: "Ada"},
		{name: "last only", first: "", middle: "", last: "Lovelace", fallback: "ignored", want: "Lovelace"},
		{name: "fallback when all empty", first: "", middle: "", last: "", fallback: "Legacy Name", want: "Legacy Name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DisplayName(tt.first, tt.middle, tt.last, tt.fallback); got != tt.want {
				t.Errorf("DisplayName(%q,%q,%q,%q) = %q, want %q", tt.first, tt.middle, tt.last, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestSplitDisplayName(t *testing.T) {
	tests := []struct {
		name                string
		input               string
		first, middle, last string
	}{
		{name: "first and last", input: "Ada Lovelace", first: "Ada", last: "Lovelace"},
		{name: "three tokens remainder to last", input: "Mary Jane Watson", first: "Mary", last: "Jane Watson"},
		{name: "single token", input: "Solo", first: "Solo"},
		{name: "extra whitespace collapsed", input: "  Ada   Lovelace  ", first: "Ada", last: "Lovelace"},
		{name: "empty", input: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first, middle, last := SplitDisplayName(tt.input)
			if first != tt.first || middle != tt.middle || last != tt.last {
				t.Errorf("SplitDisplayName(%q) = (%q,%q,%q), want (%q,%q,%q)", tt.input, first, middle, last, tt.first, tt.middle, tt.last)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	// Split then re-join must reproduce the original single-spaced name.
	for _, in := range []string{"Ada Lovelace", "Mary Jane Watson", "Solo"} {
		first, _, last := SplitDisplayName(in)
		if got := DisplayName(first, "", last, ""); got != in {
			t.Errorf("round-trip %q: got %q", in, got)
		}
	}
}
