package notifier

import "testing"

func TestPersonURL(t *testing.T) {
	tests := []struct {
		name     string
		appURL   string
		personID int
		want     string
	}{
		{name: "absolute link", appURL: "https://datey.example.com", personID: 7, want: "https://datey.example.com/people/7"},
		{name: "trailing slash collapsed", appURL: "https://datey.example.com/", personID: 7, want: "https://datey.example.com/people/7"},
		{name: "multiple trailing slashes", appURL: "http://localhost:8080//", personID: 12, want: "http://localhost:8080/people/12"},
		{name: "unset app url", appURL: "", personID: 7, want: ""},
		{name: "zero person id", appURL: "https://datey.example.com", personID: 0, want: ""},
		{name: "negative person id", appURL: "https://datey.example.com", personID: -3, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PersonURL(tt.appURL, tt.personID); got != tt.want {
				t.Errorf("PersonURL(%q, %d) = %q, want %q", tt.appURL, tt.personID, got, tt.want)
			}
		})
	}
}

func TestPersonLinkLine(t *testing.T) {
	if got := PersonLinkLine("https://datey.example.com", 7); got != "View: https://datey.example.com/people/7" {
		t.Errorf("PersonLinkLine() = %q", got)
	}
	if got := PersonLinkLine("", 7); got != "" {
		t.Errorf("PersonLinkLine() with unset APP_URL = %q, want empty", got)
	}
}
