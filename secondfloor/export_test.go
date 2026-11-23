package secondfloor

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
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

func TestRemoveTemps(t *testing.T) {
	out := t.TempDir()
	album := filepath.Join(out, "Artist", "Album")
	if err := os.MkdirAll(filepath.Join(album, tempPrefix+"dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	tmp, err := createTemp(album, ".flac")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	track := filepath.Join(album, "01_Track.flac")
	if err := os.WriteFile(track, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	removeTemps(out, slog.New(slog.DiscardHandler))

	if _, err := os.Stat(tmp.Name()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("temp file still there: %v", err)
	}
	for _, keep := range []string{track, filepath.Join(album, tempPrefix+"dir")} {
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("%s was removed: %v", keep, err)
		}
	}

	removeTemps(filepath.Join(out, "missing"), slog.New(slog.DiscardHandler))
}
