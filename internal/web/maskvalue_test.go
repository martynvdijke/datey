package web

import "testing"

func TestMaskValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"secret123", "****"},
		{"", "—"},
		{"abc", "****"},
		{" ", "****"},
	}

	for _, tt := range tests {
		t.Run("maskValue("+tt.input+")", func(t *testing.T) {
			got := maskValue(tt.input)
			if got != tt.expected {
				t.Errorf("maskValue(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"john@example.com", "j***@example.com"},
		{"", "—"},
		{"noatsign", "****"},
		{"@domain.com", "****"},
	}

	for _, tt := range tests {
		t.Run("maskEmail("+tt.input+")", func(t *testing.T) {
			got := maskEmail(tt.input)
			if got != tt.expected {
				t.Errorf("maskEmail(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
