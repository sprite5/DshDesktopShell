package host

import (
	"bufio"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// State is the lifecycle state of the managed dsh child process.
type State string

const (
	StateIdle     State = "idle"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateExited   State = "exited"
	StateFailed   State = "failed"
)

// Event is delivered to the onEvent callback on every state transition.
type Event struct {
	State State
	URL   string
	Err   error
}

// maxLogLines bounds the stderr ring kept for diagnostics.
const maxLogLines = 50

// startupTimeout is how long Start waits for the dsh URL line before failing.
const startupTimeout = 90 * time.Second

// urlLineRe matches the "dsh web: http://127.0.0.1:PORT" banner line.
var urlLineRe = regexp.MustCompile("\\bhttp://127\\.0\\.0\\.1:(\\d{1,5})\\b")

// Resolved describes how to launch the dsh CLI.
type Resolved struct {
	// Bin is the executable to invoke (a .cmd shim on Windows, an exe, or a
	// Unix binary).
	Bin string
	// Prefix holds argv that precedes the dsh flags; non-empty only for the
	// npx launcher (e.g. {"--yes", "@deepseek-ai/dsh"}).
	Prefix []string
	// Source names where the launcher was found ("env:DSH_BIN", "npm-global",
	// "path", "npx") — mainly for diagnostics.
	Source string
}

// npxArgs launches the @deepseek-ai/dsh package through npx. --yes skips the
// interactive "Ok to proceed?" install prompt, so a missing cache entry
// installs on first run instead of hanging the managed child.
var npxArgs = []string{"--yes", "@deepseek-ai/dsh"}

// Resolve locates a usable dsh launcher, trying (in order): the DSH_BIN
// environment override, the npm global bin (%APPDATA%\npm\dsh.cmd), dsh on
// PATH, then npx (@deepseek-ai/dsh via npm exec). Windows .cmd shims are
// fine — spawning routes them through cmd /c.
func Resolve() (*Resolved, error) {
	if p := os.Getenv("DSH_BIN"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return &Resolved{Bin: p, Source: "env:DSH_BIN"}, nil
		}
	}
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		for _, name := range []string{"dsh.cmd", "dsh"} {
			c := filepath.Join(appdata, "npm", name)
			if _, err := os.Stat(c); err == nil {
				return &Resolved{Bin: c, Source: "npm-global"}, nil
			}
		}
		// npx ships with npm in the same global bin dir.
		for _, name := range []string{"npx.cmd", "npx"} {
			c := filepath.Join(appdata, "npm", name)
			if _, err := os.Stat(c); err == nil {
				return &Resolved{Bin: c, Prefix: npxArgs, Source: "npx"}, nil
			}
		}
	}
	if p, err := exec.LookPath("dsh"); err == nil {
		return &Resolved{Bin: p, Source: "path"}, nil
	}
	if p, err := exec.LookPath("npx"); err == nil {
		return &Resolved{Bin: p, Prefix: npxArgs, Source: "npx"}, nil
	}
	return nil, errors.New("dsh CLI not found (npm install -g @deepseek-ai/dsh, or use npx @deepseek-ai/dsh)")
}

// DetectDSH reports whether a dsh CLI is resolvable, either directly or
// through npx.
func DetectDSH() bool {
	_, err := Resolve()
	return err == nil
}

// Host supervises one managed dsh child: spawn the resolved dsh CLI (direct
// binary or npx @deepseek-ai/dsh) with --profile web --port 0, parse the
// printed URL, and own the process lifetime (restart / kill-tree on stop,
// app exit included).
type Host struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	state   State
	url     string
	logs    []string
	onEvent func(Event)
}

// New creates a Host that reports transitions through onEvent (called from
// internal goroutines — the handler must be concurrency-safe; Wails window
// methods are). onEvent may be nil.
func New(onEvent func(Event)) *Host {
	return &Host{state: StateIdle, onEvent: onEvent}
}

func (h *Host) State() State {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state
}

func (h *Host) URL() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.url
}

// Running reports whether the child is up and its URL is known.
func (h *Host) Running() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state == StateRunning && h.url != ""
}

// Logs returns the recent stderr lines (copy).
func (h *Host) Logs() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.logs))
	copy(out, h.logs)
	return out
}

// Start spawns the dsh child and waits for its URL line (up to
// startupTimeout).
func (h *Host) Start() error {
	h.mu.Lock()
	if h.cmd != nil {
		h.mu.Unlock()
		return errors.New("dsh host already started")
	}
	h.state = StateStarting
	h.url = ""
	h.mu.Unlock()

	r, err := Resolve()
	if err != nil {
		log.Printf("[host] start: dsh not resolvable: %v", err)
		return h.fail(err)
	}
	args := append(append([]string{}, r.Prefix...), "--profile", "web", "--port", "0")
	log.Printf("[host] start: source=%s bin=%q argv=%v", r.Source, r.Bin, args)
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// .cmd shims cannot be exec'd directly; route through cmd /c.
		cmd = exec.Command("cmd", append([]string{"/c", r.Bin}, args...)...)
	} else {
		cmd = exec.Command(r.Bin, args...)
	}
	hideWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return h.fail(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return h.fail(err)
	}
	if err := cmd.Start(); err != nil {
		return h.fail(err)
	}
	h.mu.Lock()
	h.cmd = cmd
	h.mu.Unlock()

	go h.scan(stdout, true)
	go h.scan(stderr, false)
	go h.wait()

	// Await the URL line (or timeout / early exit).
	deadline := time.After(startupTimeout)
	for {
		h.mu.Lock()
		state := h.state
		url := h.url
		h.mu.Unlock()
		if state == StateRunning && url != "" {
			return nil
		}
		if state == StateExited || state == StateFailed {
			return errors.New("dsh exited before serving its URL")
		}
		select {
		case <-deadline:
			h.Stop()
			return errors.New("dsh startup timed out (no URL line)")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Restart kills the current child (if any) and starts a fresh one.
func (h *Host) Restart() error {
	h.Stop()
	// Wait until the wait() goroutine has cleared h.cmd.
	for i := 0; i < 50; i++ {
		h.mu.Lock()
		cleared := h.cmd == nil
		h.mu.Unlock()
		if cleared {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return h.Start()
}

// Stop kills the child and its whole process tree (cmd.exe wrapper spawns a
// node child on Windows).
func (h *Host) Stop() {
	h.mu.Lock()
	cmd := h.cmd
	h.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	if runtime.GOOS == "windows" {
		kc := exec.Command("taskkill", "/pid", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
		hideWindow(kc)
		_ = kc.Run()
	} else {
		_ = cmd.Process.Kill()
	}
	// wait() owns cmd.Wait() and the StateExited transition.
}

// scan reads one pipe; stdout lines are scanned for the URL banner, stderr
// lines are kept in a ring for diagnostics.
func (h *Host) scan(r io.Reader, isStdout bool) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if isStdout {
			if m := urlLineRe.FindStringSubmatch(line); m != nil {
				u := "http://127.0.0.1:" + m[1]
				h.mu.Lock()
				h.url = u
				wasStarting := h.state == StateStarting
				if wasStarting {
					h.state = StateRunning
				}
				h.mu.Unlock()
				if wasStarting {
					h.emit(Event{State: StateRunning, URL: u})
				}
			}
		} else if strings.TrimSpace(line) != "" {
			h.mu.Lock()
			h.logs = append(h.logs, line)
			if len(h.logs) > maxLogLines {
				h.logs = h.logs[len(h.logs)-maxLogLines:]
			}
			h.mu.Unlock()
		}
	}
}

// wait reaps the child and reports its exit.
func (h *Host) wait() {
	err := h.cmd.Wait()
	h.mu.Lock()
	h.cmd = nil
	wasUp := h.state == StateStarting || h.state == StateRunning
	if wasUp {
		h.state = StateExited
	}
	url := h.url
	h.mu.Unlock()
	if wasUp {
		h.emit(Event{State: StateExited, URL: url, Err: err})
	}
}

func (h *Host) fail(err error) error {
	h.mu.Lock()
	h.state = StateFailed
	h.mu.Unlock()
	h.emit(Event{State: StateFailed, Err: err})
	return err
}

func (h *Host) emit(ev Event) {
	if h.onEvent != nil {
		h.onEvent(ev)
	}
}