package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sageox/frictionax"
)

// testStore creates a Store backed by a temp SQLite database.
// The caller does NOT need to close it; cleanup is handled via t.Cleanup.
func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// testServer returns a Server wired to a fresh temp-backed Store.
func testServer(t *testing.T) *Server {
	t.Helper()
	return NewServer(testStore(t))
}

func makeEvent(kind frictionax.FailureKind, actor, input string) frictionax.FrictionEvent {
	return frictionax.FrictionEvent{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Kind:       kind,
		Command:    "mycli",
		Subcommand: "sub",
		Actor:      actor,
		AgentType:  "",
		PathBucket: "project",
		Input:      input,
		ErrorMsg:   "not found",
	}
}

// ---------------------------------------------------------------------------
// Store tests
// ---------------------------------------------------------------------------

func TestNewStore(t *testing.T) {
	t.Run("valid path", func(t *testing.T) {
		store := testStore(t)
		// schema should be initialized; verify by running a query
		count, err := store.EventCount()
		if err != nil {
			t.Fatalf("EventCount on fresh store: %v", err)
		}
		if count != 0 {
			t.Fatalf("expected 0 events, got %d", count)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		_, err := NewStore("/nonexistent/dir/test.db")
		if err == nil {
			t.Fatal("expected error for invalid path")
		}
	})
}

func TestInsertEvents(t *testing.T) {
	tests := []struct {
		name      string
		events    []frictionax.FrictionEvent
		wantCount int
	}{
		{
			name:      "empty slice inserts zero",
			events:    []frictionax.FrictionEvent{},
			wantCount: 0,
		},
		{
			name: "single event",
			events: []frictionax.FrictionEvent{
				makeEvent(frictionax.FailureUnknownCommand, "human", "gti status"),
			},
			wantCount: 1,
		},
		{
			name: "multiple events",
			events: []frictionax.FrictionEvent{
				makeEvent(frictionax.FailureUnknownCommand, "human", "gti status"),
				makeEvent(frictionax.FailureUnknownFlag, "agent", "--verboze"),
				makeEvent(frictionax.FailureInvalidArg, "human", "deploy --count=abc"),
			},
			wantCount: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := testStore(t)
			inserted, err := store.InsertEvents(tc.events)
			if err != nil {
				t.Fatalf("InsertEvents: %v", err)
			}
			if inserted != tc.wantCount {
				t.Fatalf("inserted=%d, want %d", inserted, tc.wantCount)
			}

			count, err := store.EventCount()
			if err != nil {
				t.Fatalf("EventCount: %v", err)
			}
			if count != tc.wantCount {
				t.Fatalf("EventCount=%d, want %d", count, tc.wantCount)
			}
		})
	}
}

func TestInsertEvents_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	store.Close()

	_, err = store.InsertEvents([]frictionax.FrictionEvent{
		makeEvent(frictionax.FailureUnknownCommand, "human", "foo"),
	})
	if err == nil {
		t.Fatal("expected error inserting into closed DB")
	}
}

func TestEventCount(t *testing.T) {
	store := testStore(t)

	count, err := store.EventCount()
	if err != nil {
		t.Fatalf("EventCount: %v", err)
	}
	if count != 0 {
		t.Fatalf("empty store count=%d, want 0", count)
	}

	events := []frictionax.FrictionEvent{
		makeEvent(frictionax.FailureUnknownCommand, "human", "a"),
		makeEvent(frictionax.FailureUnknownFlag, "agent", "b"),
	}
	if _, err := store.InsertEvents(events); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}

	count, err = store.EventCount()
	if err != nil {
		t.Fatalf("EventCount: %v", err)
	}
	if count != 2 {
		t.Fatalf("count=%d, want 2", count)
	}
}

func TestSummary(t *testing.T) {
	store := testStore(t)

	// seed events with timestamps in the past
	now := time.Now().UTC()
	recent := now.Add(-1 * time.Hour).Format(time.RFC3339)
	old := now.Add(-48 * time.Hour).Format(time.RFC3339)

	events := []frictionax.FrictionEvent{
		{Timestamp: recent, Kind: frictionax.FailureUnknownCommand, Actor: "human", Input: "gti status", ErrorMsg: "not found", PathBucket: "p"},
		{Timestamp: recent, Kind: frictionax.FailureUnknownCommand, Actor: "agent", Input: "gti status", ErrorMsg: "not found", PathBucket: "p"},
		{Timestamp: recent, Kind: frictionax.FailureUnknownFlag, Actor: "human", Input: "--verboze", ErrorMsg: "unknown flag", PathBucket: "p"},
		{Timestamp: old, Kind: frictionax.FailureUnknownCommand, Actor: "human", Input: "depoly", ErrorMsg: "not found", PathBucket: "p"},
	}
	if _, err := store.InsertEvents(events); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}

	t.Run("all time no filter", func(t *testing.T) {
		result, err := store.Summary("", time.Time{}, 10)
		if err != nil {
			t.Fatalf("Summary: %v", err)
		}
		if result.TotalEvents != 4 {
			t.Fatalf("TotalEvents=%d, want 4", result.TotalEvents)
		}
		if result.ByKind["unknown-command"] != 3 {
			t.Fatalf("ByKind[unknown-command]=%d, want 3", result.ByKind["unknown-command"])
		}
		if result.ByActor["human"] != 3 {
			t.Fatalf("ByActor[human]=%d, want 3", result.ByActor["human"])
		}
	})

	t.Run("filter by kind", func(t *testing.T) {
		result, err := store.Summary("unknown-flag", time.Time{}, 10)
		if err != nil {
			t.Fatalf("Summary: %v", err)
		}
		if result.TotalEvents != 1 {
			t.Fatalf("TotalEvents=%d, want 1", result.TotalEvents)
		}
	})

	t.Run("filter by time range", func(t *testing.T) {
		since := now.Add(-12 * time.Hour)
		result, err := store.Summary("", since, 10)
		if err != nil {
			t.Fatalf("Summary: %v", err)
		}
		// only 3 recent events should match
		if result.TotalEvents != 3 {
			t.Fatalf("TotalEvents=%d, want 3", result.TotalEvents)
		}
	})

	t.Run("limit constrains top inputs", func(t *testing.T) {
		result, err := store.Summary("", time.Time{}, 1)
		if err != nil {
			t.Fatalf("Summary: %v", err)
		}
		if len(result.TopInputs) > 1 {
			t.Fatalf("TopInputs len=%d, want <= 1", len(result.TopInputs))
		}
	})

	t.Run("zero limit defaults to 10", func(t *testing.T) {
		result, err := store.Summary("", time.Time{}, 0)
		if err != nil {
			t.Fatalf("Summary: %v", err)
		}
		// should not panic; just verify it returns results
		if result.TotalEvents != 4 {
			t.Fatalf("TotalEvents=%d, want 4", result.TotalEvents)
		}
	})

	t.Run("empty store returns zero values", func(t *testing.T) {
		emptyStore := testStore(t)
		result, err := emptyStore.Summary("", time.Time{}, 10)
		if err != nil {
			t.Fatalf("Summary: %v", err)
		}
		if result.TotalEvents != 0 {
			t.Fatalf("TotalEvents=%d, want 0", result.TotalEvents)
		}
		if len(result.TopInputs) != 0 {
			t.Fatalf("TopInputs should be empty")
		}
	})
}

func TestCatalog(t *testing.T) {
	store := testStore(t)

	t.Run("get from empty store returns nil", func(t *testing.T) {
		cat, err := store.GetCatalog()
		if err != nil {
			t.Fatalf("GetCatalog: %v", err)
		}
		if cat != nil {
			t.Fatalf("expected nil catalog, got %+v", cat)
		}
	})

	t.Run("set and get", func(t *testing.T) {
		catalog := &frictionax.CatalogData{
			Version: "v1",
			Commands: []frictionax.CommandMapping{
				{Pattern: "gti", Target: "git", Count: 5, Confidence: 0.95},
			},
			Tokens: []frictionax.TokenMapping{
				{Pattern: "--verboze", Target: "--verbose", Kind: frictionax.FailureUnknownFlag, Count: 3},
			},
		}
		if err := store.SetCatalog(catalog); err != nil {
			t.Fatalf("SetCatalog: %v", err)
		}

		got, err := store.GetCatalog()
		if err != nil {
			t.Fatalf("GetCatalog: %v", err)
		}
		if got == nil {
			t.Fatal("expected catalog, got nil")
		}
		if got.Version != "v1" {
			t.Fatalf("Version=%q, want %q", got.Version, "v1")
		}
		if len(got.Commands) != 1 {
			t.Fatalf("Commands len=%d, want 1", len(got.Commands))
		}
		if len(got.Tokens) != 1 {
			t.Fatalf("Tokens len=%d, want 1", len(got.Tokens))
		}
	})

	t.Run("overwrite same version", func(t *testing.T) {
		catalog := &frictionax.CatalogData{
			Version:  "v1",
			Commands: []frictionax.CommandMapping{},
			Tokens:   []frictionax.TokenMapping{},
		}
		if err := store.SetCatalog(catalog); err != nil {
			t.Fatalf("SetCatalog: %v", err)
		}
		got, err := store.GetCatalog()
		if err != nil {
			t.Fatalf("GetCatalog: %v", err)
		}
		if len(got.Commands) != 0 {
			t.Fatalf("expected empty commands after overwrite")
		}
	})

	t.Run("newer version is retrievable", func(t *testing.T) {
		// use a fresh store so created_at ordering is unambiguous
		freshStore := testStore(t)
		catalog := &frictionax.CatalogData{
			Version:  "v2",
			Commands: []frictionax.CommandMapping{{Pattern: "x", Target: "y"}},
		}
		if err := freshStore.SetCatalog(catalog); err != nil {
			t.Fatalf("SetCatalog: %v", err)
		}
		got, err := freshStore.GetCatalog()
		if err != nil {
			t.Fatalf("GetCatalog: %v", err)
		}
		if got.Version != "v2" {
			t.Fatalf("Version=%q, want %q", got.Version, "v2")
		}
		if len(got.Commands) != 1 || got.Commands[0].Target != "y" {
			t.Fatalf("expected command with target 'y', got %+v", got.Commands)
		}
	})
}

func TestPatterns(t *testing.T) {
	store := testStore(t)

	ts := time.Now().UTC().Format(time.RFC3339)
	events := []frictionax.FrictionEvent{
		{Timestamp: ts, Kind: frictionax.FailureUnknownCommand, Actor: "human", Input: "gti status", ErrorMsg: "not found", PathBucket: "p"},
		{Timestamp: ts, Kind: frictionax.FailureUnknownCommand, Actor: "human", Input: "gti status", ErrorMsg: "not found", PathBucket: "p"},
		{Timestamp: ts, Kind: frictionax.FailureUnknownCommand, Actor: "agent", AgentType: "copilot", Input: "gti status", ErrorMsg: "not found", PathBucket: "p"},
		{Timestamp: ts, Kind: frictionax.FailureUnknownFlag, Actor: "human", Input: "--verboze", ErrorMsg: "unknown flag", PathBucket: "p"},
	}
	if _, err := store.InsertEvents(events); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}

	t.Run("all patterns", func(t *testing.T) {
		patterns, err := store.Patterns(1, 100)
		if err != nil {
			t.Fatalf("Patterns: %v", err)
		}
		if len(patterns) != 2 {
			t.Fatalf("len=%d, want 2", len(patterns))
		}
		// first should be "gti status" with count 3
		if patterns[0].TotalCount != 3 {
			t.Fatalf("first pattern TotalCount=%d, want 3", patterns[0].TotalCount)
		}
		if patterns[0].HumanCount != 2 {
			t.Fatalf("HumanCount=%d, want 2", patterns[0].HumanCount)
		}
		if patterns[0].AgentCount != 1 {
			t.Fatalf("AgentCount=%d, want 1", patterns[0].AgentCount)
		}
	})

	t.Run("minCount filters", func(t *testing.T) {
		patterns, err := store.Patterns(3, 100)
		if err != nil {
			t.Fatalf("Patterns: %v", err)
		}
		if len(patterns) != 1 {
			t.Fatalf("len=%d, want 1 (only gti status >= 3)", len(patterns))
		}
	})

	t.Run("limit caps results", func(t *testing.T) {
		patterns, err := store.Patterns(1, 1)
		if err != nil {
			t.Fatalf("Patterns: %v", err)
		}
		if len(patterns) != 1 {
			t.Fatalf("len=%d, want 1", len(patterns))
		}
	})

	t.Run("empty store returns empty slice", func(t *testing.T) {
		emptyStore := testStore(t)
		patterns, err := emptyStore.Patterns(1, 100)
		if err != nil {
			t.Fatalf("Patterns: %v", err)
		}
		if len(patterns) != 0 {
			t.Fatalf("expected empty patterns")
		}
	})

	t.Run("zero limit defaults to 100", func(t *testing.T) {
		patterns, err := store.Patterns(1, 0)
		if err != nil {
			t.Fatalf("Patterns: %v", err)
		}
		if len(patterns) == 0 {
			t.Fatal("expected patterns with default limit")
		}
	})
}

// ---------------------------------------------------------------------------
// HTTP handler tests
// ---------------------------------------------------------------------------

func doRequest(t *testing.T, srv *Server, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	return rr
}

func doRequestRaw(t *testing.T, srv *Server, method, path string, rawBody []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	return rr
}

func doRequestWithHeaders(t *testing.T, srv *Server, method, path string, body interface{}, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	return rr
}

func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(rr.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, rr.Body.String())
	}
}

func TestHandlePostFriction(t *testing.T) {
	t.Run("valid submit", func(t *testing.T) {
		srv := testServer(t)
		req := frictionax.SubmitRequest{
			Version: "1.0",
			Events: []frictionax.FrictionEvent{
				makeEvent(frictionax.FailureUnknownCommand, "human", "gti status"),
			},
		}
		rr := doRequest(t, srv, "POST", "/api/v1/friction", req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d, want %d, body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}

		var resp frictionax.FrictionResponse
		decodeJSON(t, rr, &resp)
		if resp.Accepted != 1 {
			t.Fatalf("Accepted=%d, want 1", resp.Accepted)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		srv := testServer(t)
		rr := doRequestRaw(t, srv, "POST", "/api/v1/friction", []byte(`{bad json`))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("too many events", func(t *testing.T) {
		srv := testServer(t)
		events := make([]frictionax.FrictionEvent, 101)
		for i := range events {
			events[i] = makeEvent(frictionax.FailureUnknownCommand, "human", "x")
		}
		req := frictionax.SubmitRequest{Events: events}
		rr := doRequest(t, srv, "POST", "/api/v1/friction", req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("exactly 100 events accepted", func(t *testing.T) {
		srv := testServer(t)
		events := make([]frictionax.FrictionEvent, 100)
		for i := range events {
			events[i] = makeEvent(frictionax.FailureUnknownCommand, "human", "x")
		}
		req := frictionax.SubmitRequest{Events: events}
		rr := doRequest(t, srv, "POST", "/api/v1/friction", req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d, want %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("empty events accepted", func(t *testing.T) {
		srv := testServer(t)
		req := frictionax.SubmitRequest{Events: []frictionax.FrictionEvent{}}
		rr := doRequest(t, srv, "POST", "/api/v1/friction", req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d, want %d", rr.Code, http.StatusOK)
		}
		var resp frictionax.FrictionResponse
		decodeJSON(t, rr, &resp)
		if resp.Accepted != 0 {
			t.Fatalf("Accepted=%d, want 0", resp.Accepted)
		}
	})

	t.Run("returns catalog when client version differs", func(t *testing.T) {
		srv := testServer(t)
		// set a catalog first
		catalog := &frictionax.CatalogData{Version: "v2", Commands: []frictionax.CommandMapping{}, Tokens: []frictionax.TokenMapping{}}
		if err := srv.store.SetCatalog(catalog); err != nil {
			t.Fatalf("SetCatalog: %v", err)
		}

		req := frictionax.SubmitRequest{
			Version: "1.0",
			Events:  []frictionax.FrictionEvent{makeEvent(frictionax.FailureUnknownCommand, "human", "x")},
		}
		rr := doRequestWithHeaders(t, srv, "POST", "/api/v1/friction", req, map[string]string{
			"X-Catalog-Version": "v1",
		})

		var resp frictionax.FrictionResponse
		decodeJSON(t, rr, &resp)
		if resp.Catalog == nil {
			t.Fatal("expected catalog in response when version differs")
		}
		if resp.Catalog.Version != "v2" {
			t.Fatalf("catalog version=%q, want %q", resp.Catalog.Version, "v2")
		}
	})

	t.Run("omits catalog when client version matches", func(t *testing.T) {
		srv := testServer(t)
		catalog := &frictionax.CatalogData{Version: "v2", Commands: []frictionax.CommandMapping{}, Tokens: []frictionax.TokenMapping{}}
		if err := srv.store.SetCatalog(catalog); err != nil {
			t.Fatalf("SetCatalog: %v", err)
		}

		req := frictionax.SubmitRequest{
			Version: "1.0",
			Events:  []frictionax.FrictionEvent{makeEvent(frictionax.FailureUnknownCommand, "human", "x")},
		}
		rr := doRequestWithHeaders(t, srv, "POST", "/api/v1/friction", req, map[string]string{
			"X-Catalog-Version": "v2",
		})

		var resp frictionax.FrictionResponse
		decodeJSON(t, rr, &resp)
		if resp.Catalog != nil {
			t.Fatal("expected no catalog when client version matches")
		}
	})
}

func TestHandleGetStatus(t *testing.T) {
	srv := testServer(t)

	t.Run("empty store", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/v1/friction/status", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d, want %d", rr.Code, http.StatusOK)
		}
		var resp map[string]interface{}
		decodeJSON(t, rr, &resp)
		if resp["status"] != "ok" {
			t.Fatalf("status=%v, want %q", resp["status"], "ok")
		}
		if resp["event_count"].(float64) != 0 {
			t.Fatalf("event_count=%v, want 0", resp["event_count"])
		}
	})

	t.Run("with events", func(t *testing.T) {
		events := []frictionax.FrictionEvent{
			makeEvent(frictionax.FailureUnknownCommand, "human", "x"),
			makeEvent(frictionax.FailureUnknownFlag, "agent", "y"),
		}
		if _, err := srv.store.InsertEvents(events); err != nil {
			t.Fatalf("InsertEvents: %v", err)
		}

		rr := doRequest(t, srv, "GET", "/api/v1/friction/status", nil)
		var resp map[string]interface{}
		decodeJSON(t, rr, &resp)
		if resp["event_count"].(float64) != 2 {
			t.Fatalf("event_count=%v, want 2", resp["event_count"])
		}
	})
}

func TestHandleGetSummary(t *testing.T) {
	srv := testServer(t)

	// seed data
	ts := time.Now().UTC().Format(time.RFC3339)
	events := []frictionax.FrictionEvent{
		{Timestamp: ts, Kind: frictionax.FailureUnknownCommand, Actor: "human", Input: "gti", ErrorMsg: "err", PathBucket: "p"},
		{Timestamp: ts, Kind: frictionax.FailureUnknownFlag, Actor: "agent", Input: "--verboze", ErrorMsg: "err", PathBucket: "p"},
	}
	if _, err := srv.store.InsertEvents(events); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}

	t.Run("no params", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/v1/friction/summary", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d, body=%s", rr.Code, rr.Body.String())
		}
		var result SummaryResult
		decodeJSON(t, rr, &result)
		if result.TotalEvents != 2 {
			t.Fatalf("TotalEvents=%d, want 2", result.TotalEvents)
		}
	})

	t.Run("kind filter", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/v1/friction/summary?kind=unknown-flag", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d", rr.Code)
		}
		var result SummaryResult
		decodeJSON(t, rr, &result)
		if result.TotalEvents != 1 {
			t.Fatalf("TotalEvents=%d, want 1", result.TotalEvents)
		}
	})

	t.Run("since param", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/v1/friction/summary?since=1h", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d, body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("invalid since", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/v1/friction/summary?since=xyz", nil)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("limit param", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/v1/friction/summary?limit=1", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d", rr.Code)
		}
	})

	t.Run("invalid limit", func(t *testing.T) {
		tests := []struct {
			name  string
			query string
		}{
			{"non-numeric", "?limit=abc"},
			{"zero", "?limit=0"},
			{"negative", "?limit=-1"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				rr := doRequest(t, srv, "GET", "/api/v1/friction/summary"+tc.query, nil)
				if rr.Code != http.StatusBadRequest {
					t.Fatalf("status=%d, want %d", rr.Code, http.StatusBadRequest)
				}
			})
		}
	})
}

func TestHandleGetCatalog(t *testing.T) {
	t.Run("empty catalog", func(t *testing.T) {
		srv := testServer(t)
		rr := doRequest(t, srv, "GET", "/api/v1/friction/catalog", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d", rr.Code)
		}
		var resp map[string]interface{}
		decodeJSON(t, rr, &resp)
		if resp["version"] != "" {
			t.Fatalf("version=%v, want empty string", resp["version"])
		}
	})

	t.Run("existing catalog", func(t *testing.T) {
		srv := testServer(t)
		catalog := &frictionax.CatalogData{
			Version:  "v3",
			Commands: []frictionax.CommandMapping{{Pattern: "gti", Target: "git"}},
			Tokens:   []frictionax.TokenMapping{},
		}
		if err := srv.store.SetCatalog(catalog); err != nil {
			t.Fatalf("SetCatalog: %v", err)
		}

		rr := doRequest(t, srv, "GET", "/api/v1/friction/catalog", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d", rr.Code)
		}
		var resp frictionax.CatalogData
		decodeJSON(t, rr, &resp)
		if resp.Version != "v3" {
			t.Fatalf("version=%q, want %q", resp.Version, "v3")
		}
	})
}

func TestHandlePutCatalog(t *testing.T) {
	t.Run("valid catalog", func(t *testing.T) {
		srv := testServer(t)
		catalog := frictionax.CatalogData{
			Version:  "v1",
			Commands: []frictionax.CommandMapping{},
			Tokens:   []frictionax.TokenMapping{},
		}
		rr := doRequest(t, srv, "PUT", "/api/v1/friction/catalog", catalog)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d, body=%s", rr.Code, rr.Body.String())
		}
		var resp map[string]string
		decodeJSON(t, rr, &resp)
		if resp["version"] != "v1" {
			t.Fatalf("version=%q, want %q", resp["version"], "v1")
		}
	})

	t.Run("missing version", func(t *testing.T) {
		srv := testServer(t)
		catalog := frictionax.CatalogData{
			Version: "",
		}
		rr := doRequest(t, srv, "PUT", "/api/v1/friction/catalog", catalog)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		srv := testServer(t)
		rr := doRequestRaw(t, srv, "PUT", "/api/v1/friction/catalog", []byte(`not json`))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("persists and retrievable", func(t *testing.T) {
		srv := testServer(t)
		catalog := frictionax.CatalogData{
			Version:  "v5",
			Commands: []frictionax.CommandMapping{{Pattern: "depoly", Target: "deploy"}},
			Tokens:   []frictionax.TokenMapping{{Pattern: "--hlep", Target: "--help", Kind: frictionax.FailureUnknownFlag}},
		}
		rr := doRequest(t, srv, "PUT", "/api/v1/friction/catalog", catalog)
		if rr.Code != http.StatusOK {
			t.Fatalf("PUT status=%d", rr.Code)
		}

		rr = doRequest(t, srv, "GET", "/api/v1/friction/catalog", nil)
		var got frictionax.CatalogData
		decodeJSON(t, rr, &got)
		if got.Version != "v5" {
			t.Fatalf("GET version=%q, want %q", got.Version, "v5")
		}
		if len(got.Commands) != 1 || got.Commands[0].Target != "deploy" {
			t.Fatalf("commands not persisted correctly: %+v", got.Commands)
		}
	})
}

func TestHandleGetPatterns(t *testing.T) {
	srv := testServer(t)

	ts := time.Now().UTC().Format(time.RFC3339)
	events := []frictionax.FrictionEvent{
		{Timestamp: ts, Kind: frictionax.FailureUnknownCommand, Actor: "human", Input: "gti", ErrorMsg: "err", PathBucket: "p"},
		{Timestamp: ts, Kind: frictionax.FailureUnknownCommand, Actor: "human", Input: "gti", ErrorMsg: "err", PathBucket: "p"},
		{Timestamp: ts, Kind: frictionax.FailureUnknownFlag, Actor: "agent", Input: "--verboze", ErrorMsg: "err", PathBucket: "p"},
	}
	if _, err := srv.store.InsertEvents(events); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}

	t.Run("default params", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/v1/friction/patterns", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d", rr.Code)
		}
		var resp frictionax.PatternsResponse
		decodeJSON(t, rr, &resp)
		if resp.Total != 2 {
			t.Fatalf("Total=%d, want 2", resp.Total)
		}
	})

	t.Run("min_count filters", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/v1/friction/patterns?min_count=2", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d", rr.Code)
		}
		var resp frictionax.PatternsResponse
		decodeJSON(t, rr, &resp)
		if resp.Total != 1 {
			t.Fatalf("Total=%d, want 1", resp.Total)
		}
	})

	t.Run("limit caps results", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/v1/friction/patterns?limit=1", nil)
		var resp frictionax.PatternsResponse
		decodeJSON(t, rr, &resp)
		if resp.Total != 1 {
			t.Fatalf("Total=%d, want 1", resp.Total)
		}
	})

	t.Run("invalid min_count ignored", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/v1/friction/patterns?min_count=abc", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d, want %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("limit over 500 ignored", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/api/v1/friction/patterns?limit=999", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d, want %d", rr.Code, http.StatusOK)
		}
	})
}

func TestHandleImportEvents(t *testing.T) {
	t.Run("valid import", func(t *testing.T) {
		srv := testServer(t)
		body := map[string]interface{}{
			"events": []frictionax.FrictionEvent{
				makeEvent(frictionax.FailureUnknownCommand, "human", "gti"),
				makeEvent(frictionax.FailureUnknownFlag, "agent", "--verboze"),
			},
		}
		rr := doRequest(t, srv, "POST", "/api/v1/friction/import", body)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d, body=%s", rr.Code, rr.Body.String())
		}
		var resp map[string]int
		decodeJSON(t, rr, &resp)
		if resp["imported"] != 2 {
			t.Fatalf("imported=%d, want 2", resp["imported"])
		}
	})

	t.Run("empty events rejected", func(t *testing.T) {
		srv := testServer(t)
		body := map[string]interface{}{"events": []frictionax.FrictionEvent{}}
		rr := doRequest(t, srv, "POST", "/api/v1/friction/import", body)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("too many events rejected", func(t *testing.T) {
		srv := testServer(t)
		events := make([]frictionax.FrictionEvent, 1001)
		for i := range events {
			events[i] = makeEvent(frictionax.FailureUnknownCommand, "human", "x")
		}
		body := map[string]interface{}{"events": events}
		rr := doRequest(t, srv, "POST", "/api/v1/friction/import", body)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("exactly 1000 events accepted", func(t *testing.T) {
		srv := testServer(t)
		events := make([]frictionax.FrictionEvent, 1000)
		for i := range events {
			events[i] = makeEvent(frictionax.FailureUnknownCommand, "human", "x")
		}
		body := map[string]interface{}{"events": events}
		rr := doRequest(t, srv, "POST", "/api/v1/friction/import", body)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d, want %d, body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		srv := testServer(t)
		rr := doRequestRaw(t, srv, "POST", "/api/v1/friction/import", []byte(`{bad`))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d, want %d", rr.Code, http.StatusBadRequest)
		}
	})
}

func TestHandleDashboard(t *testing.T) {
	srv := testServer(t)

	t.Run("returns HTML", func(t *testing.T) {
		rr := doRequest(t, srv, "GET", "/dashboard", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d", rr.Code)
		}
		ct := rr.Header().Get("Content-Type")
		if ct != "text/html; charset=utf-8" {
			t.Fatalf("Content-Type=%q, want text/html; charset=utf-8", ct)
		}
		body := rr.Body.String()
		if len(body) < 100 {
			t.Fatal("dashboard HTML suspiciously short")
		}
	})

	t.Run("with data", func(t *testing.T) {
		ts := time.Now().UTC().Format(time.RFC3339)
		events := []frictionax.FrictionEvent{
			{Timestamp: ts, Kind: frictionax.FailureUnknownCommand, Actor: "human", Input: "gti", ErrorMsg: "err", PathBucket: "p"},
		}
		if _, err := srv.store.InsertEvents(events); err != nil {
			t.Fatalf("InsertEvents: %v", err)
		}

		rr := doRequest(t, srv, "GET", "/dashboard", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d", rr.Code)
		}
	})
}

func TestParseSinceDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		// approximate expected duration from now (we check within tolerance)
		wantDuration time.Duration
	}{
		{name: "hours", input: "24h", wantDuration: 24 * time.Hour},
		{name: "minutes", input: "30m", wantDuration: 30 * time.Minute},
		{name: "days", input: "7d", wantDuration: 7 * 24 * time.Hour},
		{name: "30 days", input: "30d", wantDuration: 30 * 24 * time.Hour},
		{name: "1 day", input: "1d", wantDuration: 24 * time.Hour},
		{name: "invalid", input: "xyz", wantErr: true},
		{name: "invalid days", input: "abcd", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSinceDuration(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			now := time.Now().UTC()
			elapsed := now.Sub(got)
			tolerance := 5 * time.Second
			if elapsed < tc.wantDuration-tolerance || elapsed > tc.wantDuration+tolerance {
				t.Fatalf("parsed time is %v ago, want ~%v ago", elapsed, tc.wantDuration)
			}
		})
	}
}

// TestContentTypeJSON verifies all JSON endpoints return proper content type.
func TestContentTypeJSON(t *testing.T) {
	srv := testServer(t)
	endpoints := []struct {
		method string
		path   string
		body   interface{}
	}{
		{"GET", "/api/v1/friction/status", nil},
		{"GET", "/api/v1/friction/summary", nil},
		{"GET", "/api/v1/friction/catalog", nil},
		{"GET", "/api/v1/friction/patterns", nil},
		{"POST", "/api/v1/friction", frictionax.SubmitRequest{Events: []frictionax.FrictionEvent{}}},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			rr := doRequest(t, srv, ep.method, ep.path, ep.body)
			ct := rr.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Fatalf("Content-Type=%q, want application/json", ct)
			}
		})
	}
}

// TestNewStoreInvalidPathError verifies the error wraps context about what failed.
func TestNewStoreInvalidPathError(t *testing.T) {
	// attempt to create store in a directory that doesn't exist
	_, err := NewStore(filepath.Join(os.TempDir(), "nonexistent-subdir-"+time.Now().Format("20060102150405"), "test.db"))
	if err == nil {
		t.Fatal("expected error for path in nonexistent directory")
	}
}
