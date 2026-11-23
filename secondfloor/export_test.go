package secondfloor

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizePathComponent(t *testing.T) {
	long := strings.Repeat("é", 300)
	tests := []struct{ in, want string }{
		{"AC/DC", "ACDC"},
		{"Who? What*", "Who_What"},
		{"", "_"},
		{".", "_"},
		{"..", "_"},
		{"Vol. 2.", "Vol._2"},
		{"CON", "_CON"},
		{"nul", "_nul"},
		{"Aux.mix", "_Aux.mix"},
		{"COM1", "_COM1"},
		{"LPT9", "_LPT9"},
		{"COM0", "COM0"},
		{"CONCERT", "CONCERT"},
	}
	for _, tt := range tests {
		if got := SanitizePathComponent(tt.in); got != tt.want {
			t.Errorf("SanitizePathComponent(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}

	got := SanitizePathComponent(long)
	if len(got) > maxComponentBytes || !utf8.ValidString(got) {
		t.Errorf("long name: %d bytes, valid utf-8 %v", len(got), utf8.ValidString(got))
	}
}
