package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"dshshell/internal/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Env bundles the dependencies the control API needs. DSH fields describe
// the managed dsh child (see internal/host); external/remote connections
// only use Store/Navigate.
type Env struct {
	Store       *settings.Store
	Version     string
	Navigate    func(url string)
	DSHDetected func() bool
	DSHResolve  func() (source string, errMsg string)
	DSHRunning  func() bool
	DSHState    func() string
	DSHLogs     func() []string
	StartDSH    func() error
}

// stateResponse is the payload served at GET /__api/state.
type stateResponse struct {
	URL          string   `json:"url"`
	Mode         string   `json:"mode"`
	Recent       []string `json:"recent"`
	Version      string   `json:"version"`
	DSHAvailable bool     `json:"dshAvailable"`
	DSHSource    string   `json:"dshSource,omitempty"`
	DSHError     string   `json:"dshError,omitempty"`
	DSHRunning   bool     `json:"dshRunning"`
	DSHState     string   `json:"dshState"`
	DSHLogs      []string `json:"dshLogs,omitempty"`
}

// connectRequest is the payload of POST /__api/connect.
type connectRequest struct {
	URL  string `json:"url"`
	Mode string `json:"mode"`
}

// probeResponse is the payload of POST /__api/probe.
type probeResponse struct {
	OK     bool   `json:"ok"`
	Status int    `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

type handlers struct {
	env Env
}

// Middleware returns an http.Handler middleware that answers the /__api/*
// control routes (settings page ↔ Go) and forwards everything else to next.
//
// These routes are only reachable while the window is on the local settings
// page: once the window navigates to a (local or remote) DSH address,
// requests go straight to that server and never touch this middleware, so
// the control API is never exposed to the DSH page or the network.
func Middleware(env Env) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		h := &handlers{env: env}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/__api/") {
				next.ServeHTTP(w, r)
				return
			}
			h.route(w, r)
		})
	}
}

func (h *handlers) route(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/__api/state":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.state(w)
	case "/__api/version":
		writeJSON(w, http.StatusOK, map[string]string{"version": h.env.Version})
	case "/__api/connect":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.connect(w, r)
	case "/__api/probe":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.probe(w, r)
	case "/__api/start-managed":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.startManaged(w)
	default:
		http.NotFound(w, r)
	}
}

func (h *handlers) state(w http.ResponseWriter) {
	detected := h.env.DSHDetected != nil && h.env.DSHDetected()
	source, errMsg := "", ""
	if h.env.DSHResolve != nil {
		source, errMsg = h.env.DSHResolve()
	}
	state := "unavailable"
	if h.env.DSHState != nil {
		state = h.env.DSHState()
	}
	writeJSON(w, http.StatusOK, stateResponse{
		URL:          h.env.Store.URL(),
		Mode:         h.env.Store.Mode(),
		Recent:       h.env.Store.Recent(),
		Version:      h.env.Version,
		DSHAvailable: detected,
		DSHSource:    source,
		DSHError:     errMsg,
		DSHRunning:   h.env.DSHRunning != nil && h.env.DSHRunning(),
		DSHState:     state,
		DSHLogs:      logsOf(h.env),
	})
}

// connect validates the submitted DSH address, persists it as an external
// connection, then asks the window to navigate there. Managed connections
// are started via /__api/start-managed instead.
func (h *handlers) connect(w http.ResponseWriter, r *http.Request) {
	var req connectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	raw := strings.TrimSpace(req.URL)
	if raw == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	url, err := application.ValidateAndSanitizeURL(raw)
	if err != nil {
		http.Error(w, "invalid url: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.env.Store.SetURL(url, settings.ModeExternal); err != nil {
		http.Error(w, "save settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if h.env.Navigate != nil {
		h.env.Navigate(url)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": url, "mode": settings.ModeExternal})
}

// startManaged launches the managed dsh child. The URL line arrives later
// through the host callback (which also persists the managed URL).
func (h *handlers) startManaged(w http.ResponseWriter) {
	if h.env.StartDSH == nil {
		http.Error(w, "managed dsh not supported here", http.StatusNotImplemented)
		return
	}
	if err := h.env.StartDSH(); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// probe performs a short connectivity check against the submitted address
// from the Go side (the settings page cannot fetch a remote origin directly
// due to CORS).
func (h *handlers) probe(w http.ResponseWriter, r *http.Request) {
	var req connectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.URL) == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, ProbeURL(req.URL))
}

// ProbeURL performs a short connectivity check (2s timeout) against the
// submitted address. Used by the settings page probe and by main at startup
// to fall back to the settings page when the saved address is unreachable.
func ProbeURL(raw string) probeResponse {
	url, err := application.ValidateAndSanitizeURL(strings.TrimSpace(raw))
	if err != nil {
		return probeResponse{OK: false, Error: "invalid url: " + err.Error()}
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return probeResponse{OK: false, Error: err.Error()}
	}
	defer resp.Body.Close()
	return probeResponse{OK: true, Status: resp.StatusCode}
}

func logsOf(env Env) []string {
	if env.DSHLogs == nil {
		return nil
	}
	return env.DSHLogs()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
