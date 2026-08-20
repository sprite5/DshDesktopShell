package host

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

func TestBuildArgs(t *testing.T) {
	direct := &Resolved{Bin: "dsh", Source: "path"}
	cases := []struct {
		noOpen bool
		r      *Resolved
		want   string
	}{
		{true, direct, "--profile web --port 0 --no-open"},
		{false, direct, "--profile web --port 0"},
		{true, &Resolved{Bin: "npx", Prefix: npxArgs, Source: "npx"}, "--yes @deepseek-ai/dsh --profile web --port 0 --no-open"},
		{false, &Resolved{Bin: "npx", Prefix: npxArgs, Source: "npx"}, "--yes @deepseek-ai/dsh --profile web --port 0"},
	}
	for _, c := range cases {
		if got := strings.Join(buildArgs(c.r, c.noOpen), " "); got != c.want {
			t.Errorf("buildArgs(%s, noOpen=%v) = %q, want %q", c.r.Source, c.noOpen, got, c.want)
		}
	}
	// buildArgs must not mutate the shared npx prefix.
	if strings.Join(npxArgs, " ") != "--yes @deepseek-ai/dsh" {
		t.Errorf("npxArgs mutated: %v", npxArgs)
	}
}

func TestNoOpenRejectedRegex(t *testing.T) {
	reject := []string{
		"error: unknown option '--no-open'",
		"Error: unknown option \"--no-open\"",
		"dsh --profile web: unrecognized flag --no-open",
		"invalid option: --no-open",
	}
	for _, line := range reject {
		if !noOpenRejectedRe.MatchString(line) {
			t.Errorf("line %q should read as a --no-open rejection", line)
		}
	}
	keep := []string{
		"dsh web: http://127.0.0.1:43210",
		"dsh web: opening the default browser; pass --no-open to disable",
		"unknown option '--port'",
	}
	for _, line := range keep {
		if noOpenRejectedRe.MatchString(line) {
			t.Errorf("line %q should not read as a --no-open rejection", line)
		}
	}
}

// fakeDSH writes a stand-in dsh CLI. With rejectNoOpen it behaves like a
// pre-rc.8 build: --no-open is an unknown option and the process exits 1.
// Otherwise it prints the URL banner and lingers like a live server.
func fakeDSH(t *testing.T, port string, rejectNoOpen bool) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		p := filepath.Join(dir, "fake-dsh.cmd")
		lines := []string{"@echo off"}
		if rejectNoOpen {
			lines = append(lines,
				`echo %*|findstr /C:"--no-open" >nul`,
				"if not errorlevel 1 goto reject")
		}
		lines = append(lines,
			"echo dsh web: http://127.0.0.1:"+port,
			"ping -n 20 127.0.0.1 >nul",
			"exit /b 0")
		if rejectNoOpen {
			lines = append(lines,
				":reject",
				`echo error: unknown option '--no-open' 1>&2`,
				"exit /b 1")
		}
		if err := os.WriteFile(p, []byte(strings.Join(lines, "\r\n")+"\r\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := filepath.Join(dir, "fake-dsh")
	lines := []string{"#!/bin/sh"}
	if rejectNoOpen {
		lines = append(lines, `case "$*" in *--no-open*) echo "error: unknown option '--no-open'" >&2; exit 1;; esac`)
	}
	lines = append(lines,
		`echo "dsh web: http://127.0.0.1:`+port+`"`,
		"sleep 20")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStartPassesNoOpenToModernCLI(t *testing.T) {
	t.Setenv("DSH_BIN", fakeDSH(t, "43211", false))
	h := New(nil)
	defer h.Stop()
	if err := h.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := h.URL(); got != "http://127.0.0.1:43211" {
		t.Errorf("URL = %q, want http://127.0.0.1:43211", got)
	}
	h.mu.Lock()
	unsupported := h.noOpenUnsupported
	h.mu.Unlock()
	if unsupported {
		t.Error("a CLI that accepted --no-open was recorded as unsupported")
	}
}

func TestStartRetriesWithoutNoOpenOnLegacyCLI(t *testing.T) {
	t.Setenv("DSH_BIN", fakeDSH(t, "43212", true))
	var mu sync.Mutex
	var states []State
	h := New(func(ev Event) {
		mu.Lock()
		states = append(states, ev.State)
		mu.Unlock()
	})
	defer h.Stop()
	if err := h.Start(); err != nil {
		t.Fatalf("Start against a pre-rc.8 CLI: %v", err)
	}
	if got := h.URL(); got != "http://127.0.0.1:43212" {
		t.Errorf("URL = %q, want http://127.0.0.1:43212", got)
	}
	h.mu.Lock()
	unsupported := h.noOpenUnsupported
	h.mu.Unlock()
	if !unsupported {
		t.Error("the rejection should be remembered so later starts skip the probe")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(states) != 1 || states[0] != StateRunning {
		t.Errorf("events = %v, want exactly [running]: the discarded probe must stay invisible", states)
	}
}
