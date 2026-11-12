package giftgap

import "testing"

func TestLine(t *testing.T) {
	tests := []struct {
		name      string
		personID  int
		openIdeas int
		appURL    string
		want      string
	}{
		{
			name:      "missing idea with link",
			personID:  7,
			openIdeas: 0,
			appURL:    "https://datey.example.com",
			want:      "No gift idea yet — add one: https://datey.example.com/people/7#gift-ideas",
		},
		{
			name:      "trailing slash on app url is collapsed",
			personID:  7,
			openIdeas: 0,
			appURL:    "https://datey.example.com/",
			want:      "No gift idea yet — add one: https://datey.example.com/people/7#gift-ideas",
		},
		{
			name:      "multiple trailing slashes are collapsed",
			personID:  12,
			openIdeas: 0,
			appURL:    "http://localhost:8080//",
			want:      "No gift idea yet — add one: http://localhost:8080/people/12#gift-ideas",
		},
		{
			name:      "blank app url yields linkless line",
			personID:  3,
			openIdeas: 0,
			appURL:    "",
			want:      "No gift idea yet — add one",
		},
		{
			name:      "open idea suppresses the line",
			personID:  7,
			openIdeas: 1,
			appURL:    "https://datey.example.com",
			want:      "",
		},
		{
			name:      "several open ideas suppress the line",
			personID:  7,
			openIdeas: 4,
			appURL:    "https://datey.example.com",
			want:      "",
		},
		{
			name:      "missing person id suppresses the line",
			personID:  0,
			openIdeas: 0,
			appURL:    "https://datey.example.com",
			want:      "",
		},
		{
			name:      "negative person id suppresses the line",
			personID:  -2,
			openIdeas: 0,
			appURL:    "https://datey.example.com",
			want:      "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Line(tt.personID, tt.openIdeas, tt.appURL); got != tt.want {
				t.Fatalf("Line(%d, %d, %q) = %q, want %q", tt.personID, tt.openIdeas, tt.appURL, got, tt.want)
			}
		})
	}
}

func TestPersonURL(t *testing.T) {
	tests := []struct {
		name     string
		personID int
		appURL   string
		want     string
	}{
		{"plain base", 5, "https://d.example", "https://d.example/people/5#gift-ideas"},
		{"trailing slash", 5, "https://d.example/", "https://d.example/people/5#gift-ideas"},
		{"empty base", 5, "", ""},
		{"invalid person", 0, "https://d.example", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PersonURL(tt.personID, tt.appURL); got != tt.want {
				t.Fatalf("PersonURL(%d, %q) = %q, want %q", tt.personID, tt.appURL, got, tt.want)
			}
		})
	}
}
