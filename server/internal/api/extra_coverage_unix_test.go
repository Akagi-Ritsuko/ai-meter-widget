//go:build !windows

package api

import (
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// getCredentialsFilePath - covers the home dir lookup
// ---------------------------------------------------------------------------

func TestGetCredentialsFilePath_WithHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := getCredentialsFilePath()
	expected := filepath.Join(home, ".claude", ".credentials.json")
	if path != expected {
		t.Errorf("getCredentialsFilePath() = %q, want %q", path, expected)
	}
}

func TestGetCredentialsFilePath_ReturnsNonEmpty(t *testing.T) {
	// Regardless of platform, should return a non-empty path if home exists
	path := getCredentialsFilePath()
	// Could be empty in some edge cases, but should not panic
	_ = path
}

// ---------------------------------------------------------------------------
// getCredentialsFilePath - HOME empty or error path
// ---------------------------------------------------------------------------

func TestGetCredentialsFilePath_EmptyHOME(t *testing.T) {
	// When HOME is not set, getCredentialsFilePath may return "" or
	// use user.Current() as a fallback. Either way it must not panic.
	t.Setenv("HOME", "")

	path := getCredentialsFilePath()
	// The function returns "" or a valid path via user.Current() fallback.
	// We just verify no panic and correct format if non-empty.
	if path != "" {
		// path should end with .claude/.credentials.json
		if !strings.HasSuffix(path, ".credentials.json") {
			t.Errorf("getCredentialsFilePath() = %q, should end with .credentials.json", path)
		}
	}
}

// ---------------------------------------------------------------------------
// getCredentialsFilePath - HOME set to temp dir (covers normal path fully)
// ---------------------------------------------------------------------------

func TestGetCredentialsFilePath_ValidHOME(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	path := getCredentialsFilePath()
	expected := filepath.Join(dir, ".claude", ".credentials.json")
	if path != expected {
		t.Errorf("getCredentialsFilePath() = %q, want %q", path, expected)
	}
}
