package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dshshell/internal/settings"
)

func testMiddleware(t *testing.T, store *settings.Store) http.Handler {
	t.Helper()
	mw := Middleware(Env{Store: store, Version: "test"})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	return mw(next)
}

func TestStateEndpoint(t *testing.T) {
	store, err := settings.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetURL("http://127.0.0.1:3080", settings.ModeExternal); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/__api/state", nil)
	rec := httptest.NewRecorder()
	testMiddleware(t, store).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var st stateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.URL != "http://127.0.0.1:3080" {
		t.Errorf("url = %q", st.URL)
	}
	if st.Version != "test" {
		t.Errorf("version = %q, want test", st.Version)
	}
}

func TestNonAPIRoutesPassThrough(t *testing.T) {
	store, _ := settings.NewStore(t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	rec := httptest.NewRecorder()
	testMiddleware(t, store).ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Errorf("passthrough status = %d, want 418", rec.Code)
	}
}

func TestConnectValidatesURL(t *testing.T) {
	store, _ := settings.NewStore(t.TempDir())
	navigated := ""
	mw := Middleware(Env{Store: store, Version: "test", Navigate: func(u string) { navigated = u }})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	// javascript: is rejected
	req := httptest.NewRequest(http.MethodPost, "/__api/connect", strings.NewReader("{\"url\":\"javascript:alert(1)\"}"))
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("javascript url status = %d, want 400", rec.Code)
	}
	if navigated != "" {
		t.Errorf("navigate called for rejected url: %q", navigated)
	}
	// http url is accepted and navigated
	req2 := httptest.NewRequest(http.MethodPost, "/__api/connect", strings.NewReader("{\"url\":\"http://127.0.0.1:9999\"}"))
	rec2 := httptest.NewRecorder()
	mw(next).ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("http url status = %d, want 200", rec2.Code)
	}
	if navigated != "http://127.0.0.1:9999" {
		t.Errorf("navigated = %q", navigated)
	}
}

func TestProbeURL(t *testing.T) {
	if res := ProbeURL("javascript:alert(1)"); res.OK {
		t.Errorf("javascript probe should fail")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	res := ProbeURL(srv.URL)
	if !res.OK || res.Status != http.StatusOK {
		t.Errorf("probe = %+v, want ok/200", res)
	}
	if res := ProbeURL("http://127.0.0.1:1"); res.OK {
		t.Errorf("unreachable probe should fail")
	}
}