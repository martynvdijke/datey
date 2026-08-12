package immich

import "testing"

func TestNormalizeName(t *testing.T) {
	if got := NormalizeName("  José O'Connor  "); got != "joséoconnor" {
		t.Fatalf("got %q", got)
	}
}

func TestExactMatchRejectsAmbiguousNames(t *testing.T) {
	people := []Person{{ID: "1", Name: "Jane Doe"}, {ID: "2", Name: "Jane-Doe"}}
	if got := ExactMatch("Jane Doe", people); got != nil {
		t.Fatalf("expected ambiguous match to be rejected: %#v", got)
	}
}
