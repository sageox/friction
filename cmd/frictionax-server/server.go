package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/sageox/frictionax"
)

// Server is the frictionax HTTP server.
type Server struct {
	store *Store
	mux   *http.ServeMux
}

// NewServer creates a new Server with the given store and registers routes.
func NewServer(store *Store) *Server {
	s := &Server{
		store: store,
		mux:   http.NewServeMux(),
	}
	s.mux.HandleFunc("POST /api/v1/friction", s.handlePostFriction)
	s.mux.HandleFunc("GET /api/v1/friction/status", s.handleGetStatus)
	s.mux.HandleFunc("GET /api/v1/friction/summary", s.handleGetSummary)
	s.mux.HandleFunc("GET /api/v1/friction/catalog", s.handleGetCatalog)
	s.mux.HandleFunc("PUT /api/v1/friction/catalog", s.handlePutCatalog)
	s.mux.HandleFunc("GET /dashboard", s.handleDashboard)
	s.mux.HandleFunc("GET /api/v1/friction/patterns", s.handleGetPatterns)
	s.mux.HandleFunc("POST /api/v1/friction/import", s.handleImportEvents)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// POST /api/v1/friction - receive friction events
func (s *Server) handlePostFriction(w http.ResponseWriter, r *http.Request) {
	var req frictionax.SubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(req.Events) > 100 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("max %d events per request", 100))
		return
	}

	// truncate event fields
	for i := range req.Events {
		req.Events[i].Truncate()
	}

	accepted, err := s.store.InsertEvents(req.Events)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store events")
		return
	}

	resp := frictionax.FrictionResponse{
		Accepted: accepted,
	}

	// check if client catalog is stale
	clientVersion := r.Header.Get("X-Catalog-Version")
	catalog, err := s.store.GetCatalog()
	if err == nil && catalog != nil && catalog.Version != clientVersion {
		resp.Catalog = catalog
	}

	writeJSON(w, http.StatusOK, resp)
}

// GET /api/v1/friction/status - health check with event count
func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	count, err := s.store.EventCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query event count")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "ok",
		"event_count": count,
	})
}

// GET /api/v1/friction/summary - aggregated friction data
func (s *Server) handleGetSummary(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")

	since := time.Time{} // default: all time
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		parsed, err := parseSinceDuration(sinceStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid since value: %s", sinceStr))
			return
		}
		since = parsed
	}

	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = parsed
	}

	summary, err := s.store.Summary(kind, since, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate summary")
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// GET /api/v1/friction/catalog - get current catalog
func (s *Server) handleGetCatalog(w http.ResponseWriter, r *http.Request) {
	catalog, err := s.store.GetCatalog()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get catalog")
		return
	}
	if catalog == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"version":  "",
			"commands": []interface{}{},
			"tokens":   []interface{}{},
		})
		return
	}
	writeJSON(w, http.StatusOK, catalog)
}

// PUT /api/v1/friction/catalog - upload catalog
func (s *Server) handlePutCatalog(w http.ResponseWriter, r *http.Request) {
	var catalog frictionax.CatalogData
	if err := json.NewDecoder(r.Body).Decode(&catalog); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if catalog.Version == "" {
		writeError(w, http.StatusBadRequest, "catalog version is required")
		return
	}

	if err := s.store.SetCatalog(&catalog); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store catalog")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": catalog.Version})
}

// parseSinceDuration parses duration strings like "24h", "7d", "30d" into a time.Time.
func parseSinceDuration(s string) (time.Time, error) {
	// handle day suffixes (not supported by time.ParseDuration)
	if len(s) > 1 && s[len(s)-1] == 'd' {
		days, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid days: %s", s)
		}
		return time.Now().UTC().AddDate(0, 0, -days), nil
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, err
	}
	return time.Now().UTC().Add(-d), nil
}

// GET /dashboard - HTML dashboard
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	summary, err := s.store.Summary("", time.Time{}, 20)
	if err != nil {
		http.Error(w, "failed to load summary", http.StatusInternalServerError)
		return
	}

	count, err := s.store.EventCount()
	if err != nil {
		http.Error(w, "failed to load event count", http.StatusInternalServerError)
		return
	}

	data := struct {
		Count   int
		Summary *SummaryResult
	}{
		Count:   count,
		Summary: summary,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := dashboardTmpl.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

var dashboardTmpl = template.Must(template.New("dashboard").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>frictionaxax dashboard</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #0f172a; color: #e2e8f0; padding: 2rem; line-height: 1.6; }
  h1 { font-size: 1.5rem; font-weight: 600; margin-bottom: 0.5rem; color: #f8fafc; }
  h2 { font-size: 1.1rem; font-weight: 500; margin: 1.5rem 0 0.75rem; color: #94a3b8; }
  .subtitle { color: #64748b; margin-bottom: 2rem; font-size: 0.9rem; }
  .stat { display: inline-block; background: #1e293b; border-radius: 8px; padding: 1rem 1.5rem; margin-right: 1rem; margin-bottom: 1rem; }
  .stat-value { font-size: 2rem; font-weight: 700; color: #38bdf8; }
  .stat-label { font-size: 0.8rem; color: #64748b; text-transform: uppercase; letter-spacing: 0.05em; }
  table { width: 100%; border-collapse: collapse; margin-bottom: 1rem; }
  th { text-align: left; padding: 0.5rem 1rem; background: #1e293b; color: #94a3b8; font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.05em; }
  td { padding: 0.5rem 1rem; border-bottom: 1px solid #1e293b; font-size: 0.9rem; }
  tr:hover td { background: #1e293b; }
  .container { max-width: 960px; margin: 0 auto; }
  code { background: #1e293b; padding: 0.15rem 0.4rem; border-radius: 4px; font-size: 0.85rem; }
</style>
</head>
<body>
<div class="container">
  <h1>frictionax</h1>
  <p class="subtitle">CLI friction telemetry dashboard. Captures usage failures (typos, unknown commands, unknown flags) to improve CLI ergonomics.</p>

  <div class="stat">
    <div class="stat-value">{{.Count}}</div>
    <div class="stat-label">Total Events</div>
  </div>

  <h2>By Kind</h2>
  <table>
    <tr><th>Kind</th><th>Count</th></tr>
    {{range $kind, $count := .Summary.ByKind}}
    <tr><td><code>{{$kind}}</code></td><td>{{$count}}</td></tr>
    {{else}}
    <tr><td colspan="2" style="color:#64748b">No events yet</td></tr>
    {{end}}
  </table>

  <h2>By Actor</h2>
  <table>
    <tr><th>Actor</th><th>Count</th></tr>
    {{range $actor, $count := .Summary.ByActor}}
    <tr><td>{{$actor}}</td><td>{{$count}}</td></tr>
    {{else}}
    <tr><td colspan="2" style="color:#64748b">No events yet</td></tr>
    {{end}}
  </table>

  <h2>Top Friction Inputs</h2>
  <table>
    <tr><th>Input</th><th>Kind</th><th>Count</th></tr>
    {{range .Summary.TopInputs}}
    <tr><td><code>{{.Input}}</code></td><td><code>{{.Kind}}</code></td><td>{{.Count}}</td></tr>
    {{else}}
    <tr><td colspan="3" style="color:#64748b">No inputs recorded yet</td></tr>
    {{end}}
  </table>
</div>
</body>
</html>`))

// GET /api/v1/friction/patterns - aggregated patterns with actor breakdown
func (s *Server) handleGetPatterns(w http.ResponseWriter, r *http.Request) {
	minCount := 1
	if v := r.URL.Query().Get("min_count"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			minCount = parsed
		}
	}

	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}

	patterns, err := s.store.Patterns(minCount, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query patterns")
		return
	}

	writeJSON(w, http.StatusOK, frictionax.PatternsResponse{
		Patterns: patterns,
		Total:    len(patterns),
	})
}

// POST /api/v1/friction/import - bulk import friction events
func (s *Server) handleImportEvents(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Events []frictionax.FrictionEvent `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(req.Events) == 0 {
		writeError(w, http.StatusBadRequest, "no events provided")
		return
	}
	if len(req.Events) > 1000 {
		writeError(w, http.StatusBadRequest, "max 1000 events per import")
		return
	}

	for i := range req.Events {
		req.Events[i].Truncate()
	}

	inserted, err := s.store.InsertEvents(req.Events)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to import events")
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"imported": inserted})
}
