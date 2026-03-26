package frictionax

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCollector_NewDefaults(t *testing.T) {
	fc := newFrictionCollector(collectorConfig{
		Endpoint: "https://api.example.com",
		Version:  "1.0.0",
	})

	if fc == nil {
		t.Fatal("newFrictionCollector returned nil")
	}
	if !fc.enabled {
		t.Error("collector should be enabled by default")
	}
	if fc.flushInterval != 15*time.Minute {
		t.Errorf("flushInterval = %v, want 15m", fc.flushInterval)
	}
	if fc.batchThreshold != 20 {
		t.Errorf("batchThreshold = %d, want 20", fc.batchThreshold)
	}
	if fc.buffer.Capacity() != 100 {
		t.Errorf("buffer capacity = %d, want 100", fc.buffer.Capacity())
	}
	if fc.catalogCache != nil {
		t.Error("catalogCache should be nil without CachePath")
	}
}

func TestCollector_NewCustomConfig(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cache.json")
	fc := newFrictionCollector(collectorConfig{
		Endpoint:       "https://api.example.com",
		Version:        "2.0.0",
		BufferSize:     50,
		FlushInterval:  5 * time.Minute,
		BatchThreshold: 10,
		FlushCooldown:  30 * time.Second,
		CachePath:      cachePath,
	})

	if fc.buffer.Capacity() != 50 {
		t.Errorf("buffer capacity = %d, want 50", fc.buffer.Capacity())
	}
	if fc.flushInterval != 5*time.Minute {
		t.Errorf("flushInterval = %v, want 5m", fc.flushInterval)
	}
	if fc.batchThreshold != 10 {
		t.Errorf("batchThreshold = %d, want 10", fc.batchThreshold)
	}
	if fc.catalogCache == nil {
		t.Error("catalogCache should not be nil when CachePath is set")
	}
}

func TestCollector_NewIsEnabled(t *testing.T) {
	tests := []struct {
		name      string
		isEnabled func() bool
		want      bool
	}{
		{
			name: "nil func defaults to enabled",
			want: true,
		},
		{
			name:      "func returns true",
			isEnabled: func() bool { return true },
			want:      true,
		},
		{
			name:      "func returns false",
			isEnabled: func() bool { return false },
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := newFrictionCollector(collectorConfig{
				Endpoint:  "https://api.example.com",
				Version:   "1.0.0",
				IsEnabled: tt.isEnabled,
			})
			if fc.IsEnabled() != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", fc.IsEnabled(), tt.want)
			}
		})
	}
}

func TestCollector_StartStop_Disabled(t *testing.T) {
	fc := newFrictionCollector(collectorConfig{
		Endpoint:  "https://api.example.com",
		Version:   "1.0.0",
		IsEnabled: func() bool { return false },
	})

	// start/stop on disabled collector should be no-ops (no panic, no hang)
	fc.Start()
	fc.Stop()
}

func TestCollector_StartStop_Enabled(t *testing.T) {
	fc := newFrictionCollector(collectorConfig{
		Endpoint:      "https://api.example.com",
		Version:       "1.0.0",
		FlushInterval: 50 * time.Millisecond,
		FlushCooldown: 1 * time.Millisecond,
	})

	fc.Start()

	// add an event so the final flush has something to process
	fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "test cmd"})

	// stop should complete without hanging
	done := make(chan struct{})
	go func() {
		fc.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not complete within timeout")
	}
}

func TestCollector_Record(t *testing.T) {
	t.Run("disabled collector drops events", func(t *testing.T) {
		fc := newFrictionCollector(collectorConfig{
			Endpoint:  "https://api.example.com",
			Version:   "1.0.0",
			IsEnabled: func() bool { return false },
		})

		fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "test"})
		if fc.buffer.Count() != 0 {
			t.Error("disabled collector should not buffer events")
		}
	})

	t.Run("adds event to buffer", func(t *testing.T) {
		fc := newFrictionCollector(collectorConfig{
			Endpoint:      "https://api.example.com",
			Version:       "1.0.0",
			FlushCooldown: time.Hour, // prevent auto-flush
		})

		fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "test cmd"})
		if fc.buffer.Count() != 1 {
			t.Errorf("buffer count = %d, want 1", fc.buffer.Count())
		}
	})

	t.Run("sets timestamp if empty", func(t *testing.T) {
		fc := newFrictionCollector(collectorConfig{
			Endpoint:      "https://api.example.com",
			Version:       "1.0.0",
			FlushCooldown: time.Hour,
		})

		before := time.Now().UTC()
		fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "no-ts"})
		events := fc.buffer.Drain()
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}

		ts, err := time.Parse(time.RFC3339, events[0].Timestamp)
		if err != nil {
			t.Fatalf("failed to parse timestamp %q: %v", events[0].Timestamp, err)
		}
		if ts.Before(before.Add(-1 * time.Second)) {
			t.Error("timestamp is too far in the past")
		}
	})

	t.Run("preserves existing timestamp", func(t *testing.T) {
		fc := newFrictionCollector(collectorConfig{
			Endpoint:      "https://api.example.com",
			Version:       "1.0.0",
			FlushCooldown: time.Hour,
		})

		custom := "2025-01-15T12:00:00Z"
		fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "has-ts", Timestamp: custom})
		events := fc.buffer.Drain()
		if events[0].Timestamp != custom {
			t.Errorf("Timestamp = %q, want %q", events[0].Timestamp, custom)
		}
	})

	t.Run("truncates long fields", func(t *testing.T) {
		fc := newFrictionCollector(collectorConfig{
			Endpoint:      "https://api.example.com",
			Version:       "1.0.0",
			FlushCooldown: time.Hour,
		})

		longInput := strings.Repeat("x", 600)
		fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: longInput})
		events := fc.buffer.Drain()
		if len(events[0].Input) != maxInputLength {
			t.Errorf("Input length = %d, want %d", len(events[0].Input), maxInputLength)
		}
	})
}

func TestCollector_IsEnabled(t *testing.T) {
	enabled := newFrictionCollector(collectorConfig{
		Endpoint: "https://api.example.com",
		Version:  "1.0.0",
	})
	if !enabled.IsEnabled() {
		t.Error("default collector should be enabled")
	}

	disabled := newFrictionCollector(collectorConfig{
		Endpoint:  "https://api.example.com",
		Version:   "1.0.0",
		IsEnabled: func() bool { return false },
	})
	if disabled.IsEnabled() {
		t.Error("disabled collector should return false")
	}
}

func TestCollector_CatalogVersion(t *testing.T) {
	t.Run("no cache returns empty", func(t *testing.T) {
		fc := newFrictionCollector(collectorConfig{
			Endpoint: "https://api.example.com",
			Version:  "1.0.0",
		})
		if v := fc.CatalogVersion(); v != "" {
			t.Errorf("CatalogVersion() = %q, want empty", v)
		}
	})

	t.Run("returns cached version", func(t *testing.T) {
		cachePath := filepath.Join(t.TempDir(), "cat.json")
		fc := newFrictionCollector(collectorConfig{
			Endpoint:  "https://api.example.com",
			Version:   "1.0.0",
			CachePath: cachePath,
		})

		// update cache with data
		fc.catalogCache.Save(&CatalogData{Version: "v42"})

		if got := fc.CatalogVersion(); got != "v42" {
			t.Errorf("CatalogVersion() = %q, want %q", got, "v42")
		}
	})
}

func TestCollector_CatalogData(t *testing.T) {
	t.Run("no cache returns nil", func(t *testing.T) {
		fc := newFrictionCollector(collectorConfig{
			Endpoint: "https://api.example.com",
			Version:  "1.0.0",
		})
		if got := fc.CatalogData(); got != nil {
			t.Errorf("CatalogData() = %v, want nil", got)
		}
	})

	t.Run("returns cached data", func(t *testing.T) {
		cachePath := filepath.Join(t.TempDir(), "cat.json")
		fc := newFrictionCollector(collectorConfig{
			Endpoint:  "https://api.example.com",
			Version:   "1.0.0",
			CachePath: cachePath,
		})
		cat := &CatalogData{
			Version: "v5",
			Tokens:  []TokenMapping{{Pattern: "a", Target: "b"}},
		}
		fc.catalogCache.Save(cat)

		got := fc.CatalogData()
		if got == nil {
			t.Fatal("CatalogData() returned nil")
		}
		if got.Version != "v5" {
			t.Errorf("Version = %q, want %q", got.Version, "v5")
		}
	})
}

func TestCollector_UpdateCatalog(t *testing.T) {
	t.Run("no cache returns false", func(t *testing.T) {
		fc := newFrictionCollector(collectorConfig{
			Endpoint: "https://api.example.com",
			Version:  "1.0.0",
		})
		changed, err := fc.UpdateCatalog(&CatalogData{Version: "v1"})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if changed {
			t.Error("UpdateCatalog without cache should return false")
		}
	})

	t.Run("delegates to cache", func(t *testing.T) {
		cachePath := filepath.Join(t.TempDir(), "cat.json")
		fc := newFrictionCollector(collectorConfig{
			Endpoint:  "https://api.example.com",
			Version:   "1.0.0",
			CachePath: cachePath,
		})

		changed, err := fc.UpdateCatalog(&CatalogData{Version: "v1"})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if !changed {
			t.Error("first UpdateCatalog should return true")
		}
		if got := fc.CatalogVersion(); got != "v1" {
			t.Errorf("CatalogVersion() = %q, want %q", got, "v1")
		}
	})
}

func TestCollector_Stats(t *testing.T) {
	fc := newFrictionCollector(collectorConfig{
		Endpoint:   "https://api.example.com",
		Version:    "1.0.0",
		BufferSize: 50,
	})

	s := fc.stats()
	if !s.Enabled {
		t.Error("stats.Enabled should be true")
	}
	if s.BufferSize != 50 {
		t.Errorf("stats.BufferSize = %d, want 50", s.BufferSize)
	}
	if s.BufferCount != 0 {
		t.Errorf("stats.BufferCount = %d, want 0", s.BufferCount)
	}
	if s.SampleRate != 1.0 {
		t.Errorf("stats.SampleRate = %v, want 1.0", s.SampleRate)
	}

	// add an event and check count
	fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "test stats"})
	s = fc.stats()
	if s.BufferCount != 1 {
		t.Errorf("stats.BufferCount = %d, want 1 after Record", s.BufferCount)
	}
}

func TestCollector_Stats_WithCatalog(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cat.json")
	fc := newFrictionCollector(collectorConfig{
		Endpoint:  "https://api.example.com",
		Version:   "1.0.0",
		CachePath: cachePath,
	})
	fc.catalogCache.Save(&CatalogData{Version: "v9"})

	s := fc.stats()
	if s.CatalogVersion != "v9" {
		t.Errorf("stats.CatalogVersion = %q, want %q", s.CatalogVersion, "v9")
	}
}

func TestCollector_Flush_SkipsWhenRateLimited(t *testing.T) {
	var requestCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		// return retry-after to trigger rate limiting
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	fc := newFrictionCollector(collectorConfig{
		Endpoint:      srv.URL,
		Version:       "1.0.0",
		FlushCooldown: 1 * time.Millisecond,
	})

	// add events and flush once to get rate limited
	fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "evt1"})
	fc.flush()

	firstCount := requestCount.Load()
	if firstCount != 1 {
		t.Fatalf("expected 1 request, got %d", firstCount)
	}

	// add more events and try to flush again -- should be skipped due to rate limit
	fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "evt2"})
	fc.flush()

	if got := requestCount.Load(); got != firstCount {
		t.Errorf("expected flush to be skipped (rate limited), but got %d requests", got)
	}
}

func TestCollector_Flush_SkipsWhenBufferEmpty(t *testing.T) {
	var requestCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fc := newFrictionCollector(collectorConfig{
		Endpoint:      srv.URL,
		Version:       "1.0.0",
		FlushCooldown: 1 * time.Millisecond,
	})

	// flush with empty buffer
	fc.flush()

	if got := requestCount.Load(); got != 0 {
		t.Errorf("expected no requests for empty buffer, got %d", got)
	}
}

func TestCollector_Flush_SendsEvents(t *testing.T) {
	var receivedReq SubmitRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedReq)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fc := newFrictionCollector(collectorConfig{
		Endpoint:      srv.URL,
		Version:       "1.0.0",
		FlushCooldown: 1 * time.Millisecond,
	})

	fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "cmd1"})
	fc.Record(FrictionEvent{Kind: FailureUnknownFlag, Input: "cmd2 --bad"})
	fc.flush()

	if len(receivedReq.Events) != 2 {
		t.Errorf("sent %d events, want 2", len(receivedReq.Events))
	}

	// buffer should be drained
	if fc.buffer.Count() != 0 {
		t.Errorf("buffer count after flush = %d, want 0", fc.buffer.Count())
	}
}

func TestCollector_Flush_UpdatesCatalogCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := FrictionResponse{
			Accepted: 1,
			Catalog: &CatalogData{
				Version: "v-new",
				Tokens:  []TokenMapping{{Pattern: "foo", Target: "bar"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "cat.json")
	fc := newFrictionCollector(collectorConfig{
		Endpoint:      srv.URL,
		Version:       "1.0.0",
		CachePath:     cachePath,
		FlushCooldown: 1 * time.Millisecond,
	})

	fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "test catalog"})
	fc.flush()

	if got := fc.CatalogVersion(); got != "v-new" {
		t.Errorf("CatalogVersion() = %q, want %q", got, "v-new")
	}
}

func TestCollector_Flush_NoCatalogUpdateOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := FrictionResponse{
			Catalog: &CatalogData{Version: "v-should-not-apply"},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "cat.json")
	fc := newFrictionCollector(collectorConfig{
		Endpoint:      srv.URL,
		Version:       "1.0.0",
		CachePath:     cachePath,
		FlushCooldown: 1 * time.Millisecond,
	})

	fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "test"})
	fc.flush()

	if got := fc.CatalogVersion(); got != "" {
		t.Errorf("CatalogVersion() = %q, want empty on non-2xx", got)
	}
}

func TestCollector_Flush_SendsCatalogVersionHeader(t *testing.T) {
	var receivedCatalogVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCatalogVersion = r.Header.Get("X-Catalog-Version")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "cat.json")
	fc := newFrictionCollector(collectorConfig{
		Endpoint:      srv.URL,
		Version:       "1.0.0",
		CachePath:     cachePath,
		FlushCooldown: 1 * time.Millisecond,
	})

	// set up catalog cache with a version
	fc.catalogCache.Save(&CatalogData{Version: "v-cached"})

	fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "test header"})
	fc.flush()

	if receivedCatalogVersion != "v-cached" {
		t.Errorf("X-Catalog-Version = %q, want %q", receivedCatalogVersion, "v-cached")
	}
}

func TestCollector_StartStop_FlushesOnStop(t *testing.T) {
	var requestCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fc := newFrictionCollector(collectorConfig{
		Endpoint:      srv.URL,
		Version:       "1.0.0",
		FlushInterval: time.Hour, // won't tick during test
		FlushCooldown: 1 * time.Millisecond,
	})

	fc.Start()
	fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "stop-flush"})
	fc.Stop()

	// the final flush in backgroundSender should have sent the event
	if got := requestCount.Load(); got == 0 {
		t.Error("expected at least one request from final flush on Stop")
	}
}

func TestCollector_Flush_WithAuth(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fc := newFrictionCollector(collectorConfig{
		Endpoint:      srv.URL,
		Version:       "1.0.0",
		AuthFunc:      func() string { return "secret-token" },
		FlushCooldown: 1 * time.Millisecond,
	})

	fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "auth test"})
	fc.flush()

	if receivedAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q, want %q", receivedAuth, "Bearer secret-token")
	}
}

func TestCollector_Flush_RequeuesOnSubmitError(t *testing.T) {
	// use an unreachable server to trigger a network error
	fc := newFrictionCollector(collectorConfig{
		Endpoint:      "http://127.0.0.1:1", // connection refused
		Version:       "1.0.0",
		FlushCooldown: 1 * time.Millisecond,
	})

	fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "net-err-1"})
	fc.Record(FrictionEvent{Kind: FailureUnknownFlag, Input: "net-err-2"})

	fc.flush()

	// events should be re-queued after network error
	if got := fc.buffer.Count(); got != 2 {
		t.Errorf("buffer count after failed flush = %d, want 2 (events should be re-queued)", got)
	}
}

func TestCollector_Flush_RequeuesOnNon2xx(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"401 unauthorized", http.StatusUnauthorized},
		{"403 forbidden", http.StatusForbidden},
		{"500 internal server error", http.StatusInternalServerError},
		{"503 service unavailable", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			fc := newFrictionCollector(collectorConfig{
				Endpoint:      srv.URL,
				Version:       "1.0.0",
				FlushCooldown: 1 * time.Millisecond,
			})

			fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "status-test"})
			fc.flush()

			if got := fc.buffer.Count(); got != 1 {
				t.Errorf("buffer count after %d response = %d, want 1 (events should be re-queued)", tt.statusCode, got)
			}
		})
	}
}

func TestCollector_Flush_DoesNotRequeueOn2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fc := newFrictionCollector(collectorConfig{
		Endpoint:      srv.URL,
		Version:       "1.0.0",
		FlushCooldown: 1 * time.Millisecond,
	})

	fc.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "success-test"})
	fc.flush()

	if got := fc.buffer.Count(); got != 0 {
		t.Errorf("buffer count after successful flush = %d, want 0", got)
	}
}
