package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageox/frictionax"
)

// helper to capture stdout during a test
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// helper to reset all package-level flag vars between tests
func resetFlags() {
	summaryKind = ""
	summarySince = ""
	summaryLimit = 10
	reportKind = ""
	reportCommand = ""
	reportSubcommand = ""
	reportActor = "human"
	reportAgentType = ""
	reportInput = ""
	reportErrorMsg = ""
	catalogFile = ""
	buildServer = ""
	buildPatternsFile = ""
	buildCatalog = "default_catalog.json"
	buildOutput = ""
	buildMinHuman = 2
	buildMinAgent = 3
	buildMinTotal = 2
	buildDiff = false
	buildFormat = "json"
}

// --- TestRunStatus ---

func TestRunStatus(t *testing.T) {
	t.Run("healthy server", func(t *testing.T) {
		resetFlags()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/friction/status" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":      "ok",
				"event_count": 42,
			})
		}))
		defer srv.Close()
		endpoint = srv.URL

		out := captureStdout(t, func() {
			err := runStatus(nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		if !strings.Contains(out, "status: ok") {
			t.Errorf("expected 'status: ok' in output, got: %s", out)
		}
		if !strings.Contains(out, "events: 42") {
			t.Errorf("expected 'events: 42' in output, got: %s", out)
		}
	})

	t.Run("server returns error status", func(t *testing.T) {
		resetFlags()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		}))
		defer srv.Close()
		endpoint = srv.URL

		err := runStatus(nil, nil)
		if err == nil {
			t.Fatal("expected error for 500 response")
		}
		if !strings.Contains(err.Error(), "server error: 500") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("server unreachable", func(t *testing.T) {
		resetFlags()
		endpoint = "http://127.0.0.1:1" // nothing listening

		err := runStatus(nil, nil)
		if err == nil {
			t.Fatal("expected error for unreachable server")
		}
		if !strings.Contains(err.Error(), "request failed") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		resetFlags()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("not json"))
		}))
		defer srv.Close()
		endpoint = srv.URL

		err := runStatus(nil, nil)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
		if !strings.Contains(err.Error(), "parse response") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// --- TestRunSummary ---

func TestRunSummary(t *testing.T) {
	t.Run("default params", func(t *testing.T) {
		resetFlags()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/friction/summary" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			// default limit=10, no kind or since
			q := r.URL.Query()
			if q.Get("limit") != "10" {
				t.Errorf("expected limit=10, got %s", q.Get("limit"))
			}
			if q.Get("kind") != "" {
				t.Errorf("expected no kind param, got %s", q.Get("kind"))
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"total_events": 100,
				"by_kind":      map[string]int{"unknown-command": 60, "parse-error": 40},
				"by_actor":     map[string]int{"human": 70, "agent": 30},
				"top_inputs": []map[string]interface{}{
					{"input": "delpoy", "count": 15, "kind": "unknown-command"},
				},
			})
		}))
		defer srv.Close()
		endpoint = srv.URL

		out := captureStdout(t, func() {
			err := runSummary(nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		if !strings.Contains(out, "total events: 100") {
			t.Errorf("expected total events in output, got: %s", out)
		}
		if !strings.Contains(out, "by kind:") {
			t.Errorf("expected 'by kind:' in output, got: %s", out)
		}
		if !strings.Contains(out, "by actor:") {
			t.Errorf("expected 'by actor:' in output, got: %s", out)
		}
		if !strings.Contains(out, "top inputs:") {
			t.Errorf("expected 'top inputs:' in output, got: %s", out)
		}
		if !strings.Contains(out, "delpoy") {
			t.Errorf("expected 'delpoy' in output, got: %s", out)
		}
	})

	t.Run("with kind and since filters", func(t *testing.T) {
		resetFlags()
		summaryKind = "unknown-command"
		summarySince = "24h"
		summaryLimit = 5

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("kind") != "unknown-command" {
				t.Errorf("expected kind=unknown-command, got %s", q.Get("kind"))
			}
			if q.Get("since") != "24h" {
				t.Errorf("expected since=24h, got %s", q.Get("since"))
			}
			if q.Get("limit") != "5" {
				t.Errorf("expected limit=5, got %s", q.Get("limit"))
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"total_events": 20,
				"by_kind":      map[string]int{},
				"by_actor":     map[string]int{},
				"top_inputs":   []interface{}{},
			})
		}))
		defer srv.Close()
		endpoint = srv.URL

		out := captureStdout(t, func() {
			err := runSummary(nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		if !strings.Contains(out, "total events: 20") {
			t.Errorf("expected 'total events: 20', got: %s", out)
		}
	})

	t.Run("server error", func(t *testing.T) {
		resetFlags()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte("bad gateway"))
		}))
		defer srv.Close()
		endpoint = srv.URL

		err := runSummary(nil, nil)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "server error: 502") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		resetFlags()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("{broken"))
		}))
		defer srv.Close()
		endpoint = srv.URL

		err := runSummary(nil, nil)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
		if !strings.Contains(err.Error(), "parse response") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// --- TestRunReport ---

func TestRunReport(t *testing.T) {
	t.Run("successful report", func(t *testing.T) {
		resetFlags()
		reportKind = "unknown-command"
		reportCommand = "git"
		reportSubcommand = ""
		reportActor = "human"
		reportInput = "delpoy"
		reportErrorMsg = "unknown command"

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/api/v1/friction" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			body, _ := io.ReadAll(r.Body)
			var req frictionax.SubmitRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			if len(req.Events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(req.Events))
			}
			if req.Events[0].Kind != frictionax.FailureUnknownCommand {
				t.Errorf("expected kind=unknown-command, got %s", req.Events[0].Kind)
			}
			if req.Events[0].Input != "delpoy" {
				t.Errorf("expected input=delpoy, got %s", req.Events[0].Input)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(frictionax.FrictionResponse{Accepted: 1})
		}))
		defer srv.Close()
		endpoint = srv.URL

		out := captureStdout(t, func() {
			err := runReport(nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		if !strings.Contains(out, "accepted: 1 event(s)") {
			t.Errorf("expected acceptance message, got: %s", out)
		}
	})

	t.Run("server error", func(t *testing.T) {
		resetFlags()
		reportKind = "parse-error"
		reportInput = "bad input"

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("unavailable"))
		}))
		defer srv.Close()
		endpoint = srv.URL

		err := runReport(nil, nil)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "server error: 503") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("long input is truncated before send", func(t *testing.T) {
		resetFlags()
		reportKind = "unknown-command"
		reportInput = strings.Repeat("x", 1000)

		var receivedInput string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var req frictionax.SubmitRequest
			json.Unmarshal(body, &req)
			if len(req.Events) > 0 {
				receivedInput = req.Events[0].Input
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(frictionax.FrictionResponse{Accepted: 1})
		}))
		defer srv.Close()
		endpoint = srv.URL

		captureStdout(t, func() {
			err := runReport(nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		if len(receivedInput) > 500 {
			t.Errorf("expected input truncated to 500 chars, got %d", len(receivedInput))
		}
	})

	t.Run("server unreachable", func(t *testing.T) {
		resetFlags()
		reportKind = "unknown-command"
		reportInput = "test"
		endpoint = "http://127.0.0.1:1"

		err := runReport(nil, nil)
		if err == nil {
			t.Fatal("expected error for unreachable server")
		}
		if !strings.Contains(err.Error(), "send request") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// --- TestRunCatalogGet ---

func TestRunCatalogGet(t *testing.T) {
	t.Run("successful get with pretty JSON", func(t *testing.T) {
		resetFlags()
		catalog := frictionax.CatalogData{
			Version: "1.0.0",
			Commands: []frictionax.CommandMapping{
				{Pattern: "delpoy", Target: "deploy", Count: 10, Confidence: 0.95},
			},
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/friction/catalog" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(catalog)
		}))
		defer srv.Close()
		endpoint = srv.URL

		out := captureStdout(t, func() {
			err := runCatalogGet(nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		// should be pretty-printed (indented)
		if !strings.Contains(out, "  ") {
			t.Errorf("expected indented JSON, got: %s", out)
		}
		if !strings.Contains(out, "1.0.0") {
			t.Errorf("expected version in output, got: %s", out)
		}
		if !strings.Contains(out, "delpoy") {
			t.Errorf("expected pattern in output, got: %s", out)
		}
	})

	t.Run("server error", func(t *testing.T) {
		resetFlags()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("forbidden"))
		}))
		defer srv.Close()
		endpoint = srv.URL

		err := runCatalogGet(nil, nil)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "server error: 403") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("non-JSON response falls back to raw output", func(t *testing.T) {
		resetFlags()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("this is not json but still 200"))
		}))
		defer srv.Close()
		endpoint = srv.URL

		out := captureStdout(t, func() {
			err := runCatalogGet(nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		if !strings.Contains(out, "this is not json but still 200") {
			t.Errorf("expected raw output fallback, got: %s", out)
		}
	})
}

// --- TestRunCatalogSet ---

func TestRunCatalogSet(t *testing.T) {
	t.Run("successful upload", func(t *testing.T) {
		resetFlags()
		catalog := frictionax.CatalogData{
			Version: "2.0.0",
			Commands: []frictionax.CommandMapping{
				{Pattern: "delpoy", Target: "deploy"},
			},
		}
		data, _ := json.Marshal(catalog)

		tmpFile := filepath.Join(t.TempDir(), "catalog.json")
		os.WriteFile(tmpFile, data, 0644)
		catalogFile = tmpFile

		var receivedMethod string
		var receivedBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method
			receivedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
		}))
		defer srv.Close()
		endpoint = srv.URL

		out := captureStdout(t, func() {
			err := runCatalogSet(nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		if receivedMethod != http.MethodPut {
			t.Errorf("expected PUT, got %s", receivedMethod)
		}
		if !strings.Contains(string(receivedBody), "2.0.0") {
			t.Errorf("expected version in body, got: %s", string(receivedBody))
		}
		if !strings.Contains(out, "catalog uploaded: version=2.0.0") {
			t.Errorf("expected upload confirmation, got: %s", out)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		resetFlags()
		catalogFile = "/nonexistent/path/catalog.json"

		err := runCatalogSet(nil, nil)
		if err == nil {
			t.Fatal("expected error for missing file")
		}
		if !strings.Contains(err.Error(), "read file") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("invalid JSON in file", func(t *testing.T) {
		resetFlags()
		tmpFile := filepath.Join(t.TempDir(), "bad.json")
		os.WriteFile(tmpFile, []byte("{not valid json"), 0644)
		catalogFile = tmpFile

		err := runCatalogSet(nil, nil)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
		if !strings.Contains(err.Error(), "invalid catalog JSON") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing version field", func(t *testing.T) {
		resetFlags()
		// valid JSON but no version
		data, _ := json.Marshal(map[string]interface{}{
			"commands": []interface{}{},
		})
		tmpFile := filepath.Join(t.TempDir(), "no-version.json")
		os.WriteFile(tmpFile, data, 0644)
		catalogFile = tmpFile

		err := runCatalogSet(nil, nil)
		if err == nil {
			t.Fatal("expected error for missing version")
		}
		if !strings.Contains(err.Error(), "catalog must have a version field") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("server error on upload", func(t *testing.T) {
		resetFlags()
		catalog := frictionax.CatalogData{Version: "1.0.0"}
		data, _ := json.Marshal(catalog)
		tmpFile := filepath.Join(t.TempDir(), "catalog.json")
		os.WriteFile(tmpFile, data, 0644)
		catalogFile = tmpFile

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte("conflict"))
		}))
		defer srv.Close()
		endpoint = srv.URL

		err := runCatalogSet(nil, nil)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "server error: 409") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// --- TestRunCatalogBuild ---

func TestRunCatalogBuild(t *testing.T) {
	t.Run("both server and file specified", func(t *testing.T) {
		resetFlags()
		buildServer = "http://example.com"
		buildPatternsFile = "patterns.json"

		err := runCatalogBuild(nil, nil)
		if err == nil {
			t.Fatal("expected error when both sources specified")
		}
		if !strings.Contains(err.Error(), "exactly one of --server or --patterns-file must be specified") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("neither server nor file specified", func(t *testing.T) {
		resetFlags()
		buildServer = ""
		buildPatternsFile = ""

		err := runCatalogBuild(nil, nil)
		if err == nil {
			t.Fatal("expected error when neither source specified")
		}
		if !strings.Contains(err.Error(), "exactly one of --server or --patterns-file must be specified") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("with patterns file", func(t *testing.T) {
		resetFlags()
		patternsData := frictionax.PatternsResponse{
			Patterns: []frictionax.PatternDetail{
				{
					Pattern:    "delpoy",
					Kind:       "unknown-command",
					TotalCount: 10,
					HumanCount: 5,
					AgentCount: 5,
				},
			},
			Total: 1,
		}
		data, _ := json.Marshal(patternsData)
		patternsFile := filepath.Join(t.TempDir(), "patterns.json")
		os.WriteFile(patternsFile, data, 0644)

		buildPatternsFile = patternsFile
		// point buildCatalog at a nonexistent file so it starts empty
		buildCatalog = filepath.Join(t.TempDir(), "nonexistent.json")

		out := captureStdout(t, func() {
			err := runCatalogBuild(nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		if !strings.Contains(out, "delpoy") {
			t.Errorf("expected pattern in output, got: %s", out)
		}
		// should contain new_entries in JSON
		if !strings.Contains(out, "new_entries") {
			t.Errorf("expected 'new_entries' key in JSON output, got: %s", out)
		}
	})

	t.Run("with server source", func(t *testing.T) {
		resetFlags()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/friction/patterns" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(frictionax.PatternsResponse{
				Patterns: []frictionax.PatternDetail{
					{
						Pattern:    "comit",
						Kind:       "unknown-command",
						TotalCount: 8,
						HumanCount: 4,
						AgentCount: 4,
					},
				},
				Total: 1,
			})
		}))
		defer srv.Close()

		buildServer = srv.URL
		buildCatalog = filepath.Join(t.TempDir(), "nonexistent.json")

		out := captureStdout(t, func() {
			err := runCatalogBuild(nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		if !strings.Contains(out, "comit") {
			t.Errorf("expected 'comit' in output, got: %s", out)
		}
	})

	t.Run("diff mode only shows new entries", func(t *testing.T) {
		resetFlags()
		// existing catalog has "delpoy"
		existingCatalog := frictionax.CatalogData{
			Version: "1.0.0",
			Commands: []frictionax.CommandMapping{
				{Pattern: "delpoy", Target: "deploy"},
			},
		}
		catalogData, _ := json.Marshal(existingCatalog)
		catalogPath := filepath.Join(t.TempDir(), "existing.json")
		os.WriteFile(catalogPath, catalogData, 0644)

		// patterns include both existing and new
		patternsData := frictionax.PatternsResponse{
			Patterns: []frictionax.PatternDetail{
				{Pattern: "delpoy", Kind: "unknown-command", TotalCount: 10, HumanCount: 5, AgentCount: 5},
				{Pattern: "comit", Kind: "unknown-command", TotalCount: 8, HumanCount: 4, AgentCount: 4},
			},
			Total: 2,
		}
		pData, _ := json.Marshal(patternsData)
		patternsFile := filepath.Join(t.TempDir(), "patterns.json")
		os.WriteFile(patternsFile, pData, 0644)

		buildPatternsFile = patternsFile
		buildCatalog = catalogPath
		buildDiff = true

		out := captureStdout(t, func() {
			err := runCatalogBuild(nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		// "comit" is new, "delpoy" should be skipped as already-in-catalog
		if !strings.Contains(out, "comit") {
			t.Errorf("expected new pattern 'comit' in output, got: %s", out)
		}

		var result frictionax.BuildResult
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
		}

		// in diff mode, catalog.commands should only be new entries
		for _, cmd := range result.Catalog.Commands {
			if cmd.Pattern == "delpoy" {
				t.Errorf("diff mode should not include existing pattern 'delpoy' in catalog commands")
			}
		}
	})

	t.Run("table format", func(t *testing.T) {
		resetFlags()
		patternsData := frictionax.PatternsResponse{
			Patterns: []frictionax.PatternDetail{
				{Pattern: "delpoy", Kind: "unknown-command", TotalCount: 10, HumanCount: 5, AgentCount: 5},
			},
			Total: 1,
		}
		data, _ := json.Marshal(patternsData)
		patternsFile := filepath.Join(t.TempDir(), "patterns.json")
		os.WriteFile(patternsFile, data, 0644)

		buildPatternsFile = patternsFile
		buildCatalog = filepath.Join(t.TempDir(), "nonexistent.json")
		buildFormat = "table"

		out := captureStdout(t, func() {
			err := runCatalogBuild(nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		if !strings.Contains(out, "NEW ENTRIES") {
			t.Errorf("expected 'NEW ENTRIES' header in table output, got: %s", out)
		}
		if !strings.Contains(out, "PATTERN") {
			t.Errorf("expected 'PATTERN' column header, got: %s", out)
		}
		if !strings.Contains(out, "delpoy") {
			t.Errorf("expected pattern in table, got: %s", out)
		}
	})

	t.Run("output to file", func(t *testing.T) {
		resetFlags()
		patternsData := frictionax.PatternsResponse{
			Patterns: []frictionax.PatternDetail{
				{Pattern: "tset", Kind: "unknown-command", TotalCount: 5, HumanCount: 3, AgentCount: 3},
			},
			Total: 1,
		}
		data, _ := json.Marshal(patternsData)
		patternsFile := filepath.Join(t.TempDir(), "patterns.json")
		os.WriteFile(patternsFile, data, 0644)

		outputFile := filepath.Join(t.TempDir(), "output.json")
		buildPatternsFile = patternsFile
		buildCatalog = filepath.Join(t.TempDir(), "nonexistent.json")
		buildOutput = outputFile

		err := runCatalogBuild(nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		written, err := os.ReadFile(outputFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		if !strings.Contains(string(written), "tset") {
			t.Errorf("expected pattern in output file, got: %s", string(written))
		}
	})
}

// --- TestFormatBuildTable ---

func TestFormatBuildTable(t *testing.T) {
	t.Run("with new entries", func(t *testing.T) {
		result := &frictionax.BuildResult{
			NewEntries: []frictionax.CommandMapping{
				{Pattern: "delpoy", Count: 10},
				{Pattern: "comit", Count: 5},
			},
		}

		out := string(formatBuildTable(result))

		if !strings.Contains(out, "NEW ENTRIES") {
			t.Errorf("expected header, got: %s", out)
		}
		if !strings.Contains(out, "delpoy") {
			t.Errorf("expected 'delpoy', got: %s", out)
		}
		if !strings.Contains(out, "comit") {
			t.Errorf("expected 'comit', got: %s", out)
		}
		if !strings.Contains(out, "PATTERN") || !strings.Contains(out, "COUNT") {
			t.Errorf("expected column headers, got: %s", out)
		}
	})

	t.Run("empty entries shows no-patterns message", func(t *testing.T) {
		result := &frictionax.BuildResult{
			NewEntries: []frictionax.CommandMapping{},
		}

		out := string(formatBuildTable(result))

		if !strings.Contains(out, "No new patterns found matching thresholds") {
			t.Errorf("expected no-patterns message, got: %s", out)
		}
	})

	t.Run("with skipped patterns", func(t *testing.T) {
		result := &frictionax.BuildResult{
			NewEntries: []frictionax.CommandMapping{
				{Pattern: "delpoy", Count: 10},
			},
			Skipped: []frictionax.SkippedPattern{
				{Pattern: "--verboze", Kind: "unknown-flag", Reason: "skipped-kind"},
				{Pattern: "existing", Kind: "unknown-command", Reason: "already-in-catalog"},
			},
		}

		out := string(formatBuildTable(result))

		if !strings.Contains(out, "SKIPPED: 2 patterns") {
			t.Errorf("expected skipped count, got: %s", out)
		}
		if !strings.Contains(out, "--verboze") {
			t.Errorf("expected skipped pattern, got: %s", out)
		}
		if !strings.Contains(out, "skipped-kind") {
			t.Errorf("expected skip reason, got: %s", out)
		}
	})

	t.Run("nil entries shows no-patterns message", func(t *testing.T) {
		result := &frictionax.BuildResult{}

		out := string(formatBuildTable(result))

		if !strings.Contains(out, "No new patterns found matching thresholds") {
			t.Errorf("expected no-patterns message, got: %s", out)
		}
	})
}
