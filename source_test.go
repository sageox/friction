package frictionax

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHTTPSource_FetchPatterns_Success(t *testing.T) {
	patterns := []PatternDetail{
		{Pattern: "statu", Kind: "unknown-command", TotalCount: 50, FirstSeen: "2024-01-01", LastSeen: "2024-06-01"},
		{Pattern: "--verbos", Kind: "unknown-flag", TotalCount: 30, FirstSeen: "2024-02-01", LastSeen: "2024-06-01"},
	}
	resp := PatternsResponse{Patterns: patterns, Total: 2}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/friction/patterns" {
			t.Errorf("path = %q, want /api/v1/friction/patterns", r.URL.Path)
		}
		// verify query params are passed
		q := r.URL.Query()
		if q.Get("min_count") != "10" {
			t.Errorf("min_count = %q, want 10", q.Get("min_count"))
		}
		if q.Get("limit") != "100" {
			t.Errorf("limit = %q, want 100", q.Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	source := NewHTTPSource(srv.URL)
	got, err := source.FetchPatterns(context.Background(), 10, 100)
	if err != nil {
		t.Fatalf("FetchPatterns error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d patterns, want 2", len(got))
	}
	if got[0].Pattern != "statu" {
		t.Errorf("patterns[0].Pattern = %q, want %q", got[0].Pattern, "statu")
	}
	if got[1].TotalCount != 30 {
		t.Errorf("patterns[1].TotalCount = %d, want 30", got[1].TotalCount)
	}
}

func TestHTTPSource_FetchPatterns_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	source := NewHTTPSource(srv.URL)
	_, err := source.FetchPatterns(context.Background(), 1, 10)
	if err == nil {
		t.Fatal("expected error for server 500 response")
	}
	if !contains(err.Error(), "500") {
		t.Errorf("error should contain status code 500, got: %v", err)
	}
}

func TestHTTPSource_FetchPatterns_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not-json{{{"))
	}))
	defer srv.Close()

	source := NewHTTPSource(srv.URL)
	_, err := source.FetchPatterns(context.Background(), 1, 10)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !contains(err.Error(), "decode") {
		t.Errorf("error should mention decode, got: %v", err)
	}
}

func TestHTTPSource_FetchPatterns_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PatternsResponse{Patterns: nil, Total: 0})
	}))
	defer srv.Close()

	source := NewHTTPSource(srv.URL)
	got, err := source.FetchPatterns(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 patterns, got %d", len(got))
	}
}

func TestHTTPSource_FetchPatterns_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(PatternsResponse{})
	}))
	defer srv.Close()

	source := NewHTTPSource(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := source.FetchPatterns(ctx, 1, 10)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

// --- FileSource tests ---

func TestFileSource_FetchPatterns_Success(t *testing.T) {
	data := PatternsResponse{
		Patterns: []PatternDetail{
			{Pattern: "statu", Kind: "unknown-command", TotalCount: 50},
			{Pattern: "depliy", Kind: "unknown-command", TotalCount: 5},
			{Pattern: "--verbos", Kind: "unknown-flag", TotalCount: 100},
		},
		Total: 3,
	}

	tmpFile := writeTestJSON(t, data)

	source := NewFileSource(tmpFile)
	got, err := source.FetchPatterns(context.Background(), 1, 0)
	if err != nil {
		t.Fatalf("FetchPatterns error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d patterns, want 3", len(got))
	}
}

func TestFileSource_FetchPatterns_MinCountFilter(t *testing.T) {
	data := PatternsResponse{
		Patterns: []PatternDetail{
			{Pattern: "statu", TotalCount: 50},
			{Pattern: "depliy", TotalCount: 5},
			{Pattern: "--verbos", TotalCount: 100},
		},
	}

	tmpFile := writeTestJSON(t, data)

	source := NewFileSource(tmpFile)
	got, err := source.FetchPatterns(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("FetchPatterns error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d patterns after minCount filter, want 2", len(got))
	}
	// only patterns with TotalCount >= 10 should remain
	for _, p := range got {
		if p.TotalCount < 10 {
			t.Errorf("pattern %q with count %d should have been filtered", p.Pattern, p.TotalCount)
		}
	}
}

func TestFileSource_FetchPatterns_LimitFilter(t *testing.T) {
	data := PatternsResponse{
		Patterns: []PatternDetail{
			{Pattern: "a", TotalCount: 100},
			{Pattern: "b", TotalCount: 90},
			{Pattern: "c", TotalCount: 80},
			{Pattern: "d", TotalCount: 70},
		},
	}

	tmpFile := writeTestJSON(t, data)

	source := NewFileSource(tmpFile)
	got, err := source.FetchPatterns(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("FetchPatterns error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d patterns after limit, want 2", len(got))
	}
	if got[0].Pattern != "a" || got[1].Pattern != "b" {
		t.Errorf("limit should keep first 2 patterns, got %q and %q", got[0].Pattern, got[1].Pattern)
	}
}

func TestFileSource_FetchPatterns_MinCountAndLimit(t *testing.T) {
	data := PatternsResponse{
		Patterns: []PatternDetail{
			{Pattern: "a", TotalCount: 100},
			{Pattern: "b", TotalCount: 2}, // filtered by minCount
			{Pattern: "c", TotalCount: 80},
			{Pattern: "d", TotalCount: 70},
		},
	}

	tmpFile := writeTestJSON(t, data)

	source := NewFileSource(tmpFile)
	got, err := source.FetchPatterns(context.Background(), 10, 2)
	if err != nil {
		t.Fatalf("FetchPatterns error: %v", err)
	}
	// after minCount filter: a(100), c(80), d(70) = 3, then limit to 2
	if len(got) != 2 {
		t.Fatalf("got %d patterns, want 2", len(got))
	}
	if got[0].Pattern != "a" || got[1].Pattern != "c" {
		t.Errorf("expected a and c, got %q and %q", got[0].Pattern, got[1].Pattern)
	}
}

func TestFileSource_FetchPatterns_FileNotFound(t *testing.T) {
	source := NewFileSource("/nonexistent/path/to/file.json")
	_, err := source.FetchPatterns(context.Background(), 1, 10)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !contains(err.Error(), "open file") {
		t.Errorf("error should mention opening file, got: %v", err)
	}
}

func TestFileSource_FetchPatterns_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(tmpFile, []byte("{not valid json!!!"), 0644); err != nil {
		t.Fatal(err)
	}

	source := NewFileSource(tmpFile)
	_, err := source.FetchPatterns(context.Background(), 1, 10)
	if err == nil {
		t.Fatal("expected error for invalid JSON file")
	}
}

func TestFileSource_FetchPatterns_EmptyPatterns(t *testing.T) {
	data := PatternsResponse{Patterns: []PatternDetail{}, Total: 0}
	tmpFile := writeTestJSON(t, data)

	source := NewFileSource(tmpFile)
	got, err := source.FetchPatterns(context.Background(), 1, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 patterns, got %d", len(got))
	}
}

func TestFileSource_FetchPatterns_LimitZeroMeansNoLimit(t *testing.T) {
	data := PatternsResponse{
		Patterns: []PatternDetail{
			{Pattern: "a", TotalCount: 10},
			{Pattern: "b", TotalCount: 20},
			{Pattern: "c", TotalCount: 30},
		},
	}
	tmpFile := writeTestJSON(t, data)

	source := NewFileSource(tmpFile)
	got, err := source.FetchPatterns(context.Background(), 1, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("limit=0 should return all, got %d patterns", len(got))
	}
}

// helper to write JSON to a temp file
func writeTestJSON(t *testing.T, data any) string {
	t.Helper()
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "patterns.json")
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmpFile, raw, 0644); err != nil {
		t.Fatal(err)
	}
	return tmpFile
}
