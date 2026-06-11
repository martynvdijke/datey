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
