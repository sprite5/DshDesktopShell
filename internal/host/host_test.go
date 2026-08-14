package host

import (
	"os"
	"path/filepath"
	"testing"
)

func TestURLRegex(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{"dsh web: http://127.0.0.1:43210", "http://127.0.0.1:43210"},
		{"dsh web: http://127.0.0.1:43210 (LAN: http://192.168.1.5:43210)", "http://127.0.0.1:43210"},
		{"  http://127.0.0.1:7  ", "http://127.0.0.1:7"},
		{"no url here", ""},
		{"https://127.0.0.1:9", ""}, // only http loopback banner
	}
	for _, c := range cases {
		m := urlLineRe.FindStringSubmatch(c.line)
		got := ""
		if m != nil {
			got = "http://127.0.0.1:" + m[1]
		}
		if got != c.want {
			t.Errorf("line %q: got %q, want %q", c.line, got, c.want)
		}
	}
}

func TestDetectDSH(t *testing.T) {
	// Environment-dependent; just assert it runs without panicking.
	_ = DetectDSH()
}

func TestResolvePrefersDSHBin(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "dsh")
	if err := os.WriteFile(bin, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DSH_BIN", bin)
	r, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.Bin != bin || r.Source != "env:DSH_BIN" {
		t.Errorf("got Bin=%q Source=%q, want %q/env:DSH_BIN", r.Bin, r.Source, bin)
	}
	if len(r.Prefix) != 0 {
		t.Errorf("DSH_BIN should carry no prefix, got %v", r.Prefix)
	}
}

func TestResolveNpxFromAppData(t *testing.T) {
	dir := t.TempDir()
	npm := filepath.Join(dir, "npm")
	if err := os.MkdirAll(npm, 0o755); err != nil {
		t.Fatal(err)
	}
	npx := filepath.Join(npm, "npx.cmd")
	if err := os.WriteFile(npx, []byte("@echo off"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DSH_BIN", "")
	t.Setenv("APPDATA", dir)
	t.Setenv("PATH", dir) // block PATH dsh/npx lookups
	r, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.Bin != npx || r.Source != "npx" {
		t.Errorf("got Bin=%q Source=%q, want %q/npx", r.Bin, r.Source, npx)
	}
	if len(r.Prefix) != 2 || r.Prefix[0] != "--yes" || r.Prefix[1] != "@deepseek-ai/dsh" {
		t.Errorf("npx prefix = %v, want [--yes @deepseek-ai/dsh]", r.Prefix)
	}
}

func TestResolveNone(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSH_BIN", "")
	t.Setenv("APPDATA", dir) // no npm subdir inside
	t.Setenv("PATH", dir)    // nothing resolvable there
	if _, err := Resolve(); err == nil {
		t.Fatal("Resolve succeeded, want error")
	}
}
