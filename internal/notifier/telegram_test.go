package notifier

import (
	"testing"

	"github.com/datey/datey/internal/config"
)

func TestTelegramNotifier_Name(t *testing.T) {
	n := NewTelegramNotifier(&config.Config{})
	if got := n.Name(); got != "telegram" {
		t.Errorf("Name() = %q, want %q", got, "telegram")
	}
}

func TestTelegramNotifier_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"both set", &config.Config{TelegramBotToken: "bot:token", TelegramChatID: "123"}, true},
		{"missing bot token", &config.Config{TelegramBotToken: "", TelegramChatID: "123"}, false},
		{"missing chat id", &config.Config{TelegramBotToken: "bot:token", TelegramChatID: ""}, false},
		{"both empty", &config.Config{TelegramBotToken: "", TelegramChatID: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewTelegramNotifier(tt.cfg)
			if got := n.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}
