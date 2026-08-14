package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewStoreDefaults(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if s.URL() != "" {
		t.Errorf("fresh store URL = %q, want empty", s.URL())
	}
	if w := s.Window(); w.Width != 1360 || w.Height != 860 {
		t.Errorf("default window = %+v, want 1360x860", w)
	}
	if filepathExists(filepath.Join(dir, "settings.json")) {
		t.Errorf("settings.json should not exist before first save")
	}
}

func TestSetURLPersistsAndPromotesRecents(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	for _, u := range []string{"http://a", "http://b", "http://a", "http://c"} {
		if err := s.SetURL(u, ModeExternal); err != nil {
			t.Fatalf("SetURL(%q): %v", u, err)
		}
	}
	if got := s.URL(); got != "http://c" {
		t.Errorf("URL = %q, want http://c", got)
	}
	if m := s.Mode(); m != ModeExternal {
		t.Errorf("Mode = %q, want external", m)
	}
	want := []string{"http://c", "http://a", "http://b"}
	got := s.Recent()
	if len(got) != len(want) {
		t.Fatalf("recent len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("recent[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// reload from disk
	s2, err := NewStore(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if s2.URL() != "http://c" {
		t.Errorf("reloaded URL = %q, want http://c", s2.URL())
	}
}

func TestRecentsBounded(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		if err := s.SetURL("http://u"+string(rune('a'+i)), ModeExternal); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(s.Recent()); n > maxRecent {
		t.Errorf("recents = %d, want <= %d", n, maxRecent)
	}
}

func TestResetURL(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetURL("http://keep", ModeExternal); err != nil {
		t.Fatal(err)
	}
	if err := s.SetURL("http://drop", ModeExternal); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetURL(); err != nil {
		t.Fatal(err)
	}
	if s.URL() != "" {
		t.Errorf("URL after reset = %q, want empty", s.URL())
	}
	// recents survive
	if len(s.Recent()) != 2 {
		t.Errorf("recents after reset = %v, want both kept", s.Recent())
	}
}

func filepathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}