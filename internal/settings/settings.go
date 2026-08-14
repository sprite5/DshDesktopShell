package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// WindowState holds persisted window geometry so the shell restores the
// user's last arrangement on the next launch.
type WindowState struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Connection modes. Managed = the shell spawns/owns a local dsh process
// (restartable, dies with the app). External = a user-run local dsh or a
// remote reverse proxy the shell only points at.
const (
	ModeManaged  = "managed"
	ModeExternal = "external"
)

// Settings is the persisted shell configuration.
type Settings struct {
	// URL is the last connected DSH Web address; "" means never connected.
	URL string `json:"url"`
	// Mode is ModeManaged or ModeExternal (legacy files default to external).
	Mode string `json:"mode"`
	// Recent holds recently used addresses, most-recent first (bounded).
	Recent []string `json:"recent"`
	// Window is the saved main-window geometry.
	Window WindowState `json:"window"`
}

// maxRecent bounds the recents list.
const maxRecent = 5

// Store persists Settings to a JSON file (tmp-file + rename, never in place).
type Store struct {
	mu   sync.Mutex
	path string
	cfg  Settings
}

// NewStore loads (or initialises) the settings file at dir/settings.json.
func NewStore(dir string) (*Store, error) {
	path := filepath.Join(dir, "settings.json")
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &s.cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	case os.IsNotExist(err):
		// fresh install: defaults apply
	default:
		return nil, err
	}
	s.normalize()
	return s, nil
}

func (s *Store) normalize() {
	if s.cfg.Mode != ModeManaged {
		s.cfg.Mode = ModeExternal
	}
	if s.cfg.Window.Width <= 0 {
		s.cfg.Window.Width = 1360
	}
	if s.cfg.Window.Height <= 0 {
		s.cfg.Window.Height = 860
	}
	// de-dup recents, drop empties, bound length
	seen := make(map[string]bool, len(s.cfg.Recent))
	out := make([]string, 0, len(s.cfg.Recent))
	for _, u := range s.cfg.Recent {
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	if len(out) > maxRecent {
		out = out[:maxRecent]
	}
	s.cfg.Recent = out
}

// URL returns the last connected address ("" when never connected).
func (s *Store) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.URL
}

// Recent returns the recents list (most-recent first).
func (s *Store) Recent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.cfg.Recent))
	copy(out, s.cfg.Recent)
	return out
}

// Window returns the saved window geometry.
func (s *Store) Window() WindowState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Window
}

// Mode returns the current connection mode.
func (s *Store) Mode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Mode
}

// SetURL records the active address (and its mode) and promotes it in
// recents.
func (s *Store) SetURL(url, mode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.URL = url
	s.cfg.Mode = mode
	recent := []string{url}
	for _, u := range s.cfg.Recent {
		if u == url {
			continue
		}
		recent = append(recent, u)
	}
	if len(recent) > maxRecent {
		recent = recent[:maxRecent]
	}
	s.cfg.Recent = recent
	return s.saveLocked()
}

// SetWindow persists the window geometry.
func (s *Store) SetWindow(w WindowState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Window = w
	return s.saveLocked()
}

// ResetURL clears the active address (keeps recents) — used by --reset.
func (s *Store) ResetURL() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.URL = ""
	return s.saveLocked()
}

// saveLocked writes the file atomically. Callers hold s.mu.

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(&s.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}