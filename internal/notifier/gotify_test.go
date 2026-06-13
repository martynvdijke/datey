package notifier

import (
	"testing"

	"github.com/datey/datey/internal/config"
)

func TestGotifyNotifier_Name(t *testing.T) {
	n := NewGotifyNotifier(&config.Config{})
	if got := n.Name(); got != "gotify" {
		t.Errorf("Name() = %q, want %q", got, "gotify")
	}
}

func TestGotifyNotifier_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"both set", &config.Config{GotifyURL: "http://gotify:8080", GotifyToken: "secret"}, true},
		{"missing url", &config.Config{GotifyURL: "", GotifyToken: "secret"}, false},
		{"missing token", &config.Config{GotifyURL: "http://gotify:8080", GotifyToken: ""}, false},
		{"both empty", &config.Config{GotifyURL: "", GotifyToken: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewGotifyNotifier(tt.cfg)
			if got := n.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}
