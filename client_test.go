package frictionax

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name           string
		cfg            clientConfig
		wantSampleRate float64
	}{
		{
			name: "default config",
			cfg: clientConfig{
				Endpoint: "https://api.example.com",
				Version:  "1.0.0",
			},
			wantSampleRate: 1.0,
		},
		{
			name: "with auth func",
			cfg: clientConfig{
				Endpoint: "https://api.example.com",
				Version:  "1.0.0",
				AuthFunc: func() string { return "token123" },
			},
			wantSampleRate: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newClient(tt.cfg)

			if c == nil {
				t.Fatal("newClient returned nil")
			}

			if c.SampleRate() != tt.wantSampleRate {
				t.Errorf("SampleRate() = %v, want %v", c.SampleRate(), tt.wantSampleRate)
			}

			if c.config.Timeout == 0 {
				t.Error("Timeout should have default value")
			}
		})
	}
}

func TestClient_Submit(t *testing.T) {
	tests := []struct {
		name           string
		events         []FrictionEvent
		serverStatus   int
		serverHeaders  map[string]string
		wantEventCount int
		wantErr        bool
	}{
		{
			name:           "empty events (heartbeat)",
			events:         []FrictionEvent{},
			serverStatus:   http.StatusOK,
			wantEventCount: 0,
		},
		{
			name: "single event",
			events: []FrictionEvent{
				{Kind: "unknown-command", Input: "mycli foo"},
			},
			serverStatus:   http.StatusOK,
			wantEventCount: 1,
		},
		{
			name: "multiple events",
			events: []FrictionEvent{
				{Kind: "unknown-command", Input: "mycli foo"},
				{Kind: "unknown-flag", Input: "mycli bar --baz"},
			},
			serverStatus:   http.StatusOK,
			wantEventCount: 2,
		},
		{
			name: "server returns sample rate",
			events: []FrictionEvent{
				{Kind: "unknown-command", Input: "mycli foo"},
			},
			serverStatus:  http.StatusOK,
			serverHeaders: map[string]string{"X-Friction-Sample-Rate": "0.5"},
		},
		{
			name: "server returns retry-after",
			events: []FrictionEvent{
				{Kind: "unknown-command", Input: "mycli foo"},
			},
			serverStatus:  http.StatusTooManyRequests,
			serverHeaders: map[string]string{"Retry-After": "60"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedReq SubmitRequest

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("Method = %v, want POST", r.Method)
				}
				if r.URL.Path != "/api/v1/cli/friction" {
					t.Errorf("Path = %v, want /api/v1/cli/friction", r.URL.Path)
				}
				if ct := r.Header.Get("Content-Type"); ct != "application/json" {
					t.Errorf("Content-Type = %v, want application/json", ct)
				}

				json.NewDecoder(r.Body).Decode(&receivedReq)

				for k, v := range tt.serverHeaders {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.serverStatus)
			}))
			defer server.Close()

			c := newClient(clientConfig{
				Endpoint: server.URL,
				Version:  "0.5.0",
			})

			resp, err := c.Submit(context.Background(), tt.events, nil)

			if tt.wantErr {
				if err == nil {
					t.Error("Submit() should have returned error")
				}
				return
			}

			if err != nil {
				t.Fatalf("Submit() error = %v", err)
			}

			if resp.StatusCode != tt.serverStatus {
				t.Errorf("StatusCode = %v, want %v", resp.StatusCode, tt.serverStatus)
			}

			if len(receivedReq.Events) != len(tt.events) {
				t.Errorf("Sent %d events, want %d", len(receivedReq.Events), len(tt.events))
			}

			if receivedReq.Version != "0.5.0" {
				t.Errorf("Version = %v, want 0.5.0", receivedReq.Version)
			}
		})
	}
}

func TestClient_Submit_Truncation(t *testing.T) {
	var receivedReq SubmitRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedReq)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := newClient(clientConfig{
		Endpoint: server.URL,
		Version:  "0.5.0",
	})

	longInput := strings.Repeat("x", 600)
	longError := strings.Repeat("e", 300)

	events := []FrictionEvent{
		{Kind: "unknown-command", Input: longInput, ErrorMsg: longError},
	}

	_, err := c.Submit(context.Background(), events, nil)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	if len(receivedReq.Events[0].Input) != maxInputLength {
		t.Errorf("Input length = %d, want %d", len(receivedReq.Events[0].Input), maxInputLength)
	}
	if len(receivedReq.Events[0].ErrorMsg) != maxErrorLength {
		t.Errorf("ErrorMsg length = %d, want %d", len(receivedReq.Events[0].ErrorMsg), maxErrorLength)
	}
}

func TestClient_Submit_MaxEvents(t *testing.T) {
	var receivedReq SubmitRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedReq)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := newClient(clientConfig{
		Endpoint: server.URL,
		Version:  "0.5.0",
	})

	events := make([]FrictionEvent, 150)
	for i := range events {
		events[i] = FrictionEvent{Kind: "unknown-command", Input: "test"}
	}

	_, err := c.Submit(context.Background(), events, nil)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	if len(receivedReq.Events) != maxEventsPerRequest {
		t.Errorf("Events count = %d, want %d", len(receivedReq.Events), maxEventsPerRequest)
	}
}

func TestClient_Submit_WithAuth(t *testing.T) {
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := newClient(clientConfig{
		Endpoint: server.URL,
		Version:  "0.5.0",
		AuthFunc: func() string { return "test-token-123" },
	})

	_, err := c.Submit(context.Background(), []FrictionEvent{}, nil)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	if receivedAuth != "Bearer test-token-123" {
		t.Errorf("Authorization = %v, want Bearer test-token-123", receivedAuth)
	}
}

func TestClient_ShouldSend(t *testing.T) {
	tests := []struct {
		name       string
		sampleRate float64
		retryAfter time.Time
		wantSend   bool
	}{
		{name: "default rate allows send", sampleRate: 1.0, wantSend: true},
		{name: "zero rate blocks send", sampleRate: 0.0, wantSend: false},
		{name: "retry-after in future blocks send", sampleRate: 1.0, retryAfter: time.Now().Add(1 * time.Hour), wantSend: false},
		{name: "retry-after in past allows send", sampleRate: 1.0, retryAfter: time.Now().Add(-1 * time.Hour), wantSend: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newClient(clientConfig{
				Endpoint: "https://api.example.com",
				Version:  "0.5.0",
			})

			c.mu.Lock()
			c.sampleRate = tt.sampleRate
			c.retryAfter = tt.retryAfter
			c.mu.Unlock()

			got := c.ShouldSend()
			if got != tt.wantSend {
				t.Errorf("ShouldSend() = %v, want %v", got, tt.wantSend)
			}
		})
	}
}

func TestClient_UpdateFromHeaders(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		wantSampleRate float64
		wantRetryAfter bool
	}{
		{name: "no headers", headers: map[string]string{}, wantSampleRate: 1.0},
		{name: "sample rate header", headers: map[string]string{"X-Friction-Sample-Rate": "0.25"}, wantSampleRate: 0.25},
		{name: "retry-after seconds", headers: map[string]string{"Retry-After": "120"}, wantSampleRate: 1.0, wantRetryAfter: true},
		{
			name: "both headers",
			headers: map[string]string{
				"X-Friction-Sample-Rate": "0.5",
				"Retry-After":           "60",
			},
			wantSampleRate: 0.5,
			wantRetryAfter: true,
		},
		{name: "invalid sample rate ignored", headers: map[string]string{"X-Friction-Sample-Rate": "invalid"}, wantSampleRate: 1.0},
		{name: "sample rate out of range ignored", headers: map[string]string{"X-Friction-Sample-Rate": "1.5"}, wantSampleRate: 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tt.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			c := newClient(clientConfig{
				Endpoint: server.URL,
				Version:  "0.5.0",
			})

			_, err := c.Submit(context.Background(), []FrictionEvent{}, nil)
			if err != nil {
				t.Fatalf("Submit() error = %v", err)
			}

			if c.SampleRate() != tt.wantSampleRate {
				t.Errorf("SampleRate() = %v, want %v", c.SampleRate(), tt.wantSampleRate)
			}

			hasRetryAfter := !c.RetryAfter().IsZero()
			if hasRetryAfter != tt.wantRetryAfter {
				t.Errorf("RetryAfter set = %v, want %v", hasRetryAfter, tt.wantRetryAfter)
			}
		})
	}
}

func TestClient_Reset(t *testing.T) {
	c := newClient(clientConfig{
		Endpoint: "https://api.example.com",
		Version:  "0.5.0",
	})

	c.mu.Lock()
	c.sampleRate = 0.1
	c.retryAfter = time.Now().Add(1 * time.Hour)
	c.mu.Unlock()

	if c.SampleRate() == 1.0 {
		t.Error("SampleRate should have been modified")
	}

	c.Reset()

	if c.SampleRate() != 1.0 {
		t.Errorf("SampleRate() after Reset = %v, want 1.0", c.SampleRate())
	}
	if !c.RetryAfter().IsZero() {
		t.Error("RetryAfter should be zero after Reset")
	}
}

func TestClient_Submit_SendsClientVersionHeader(t *testing.T) {
	var receivedVersion string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedVersion = r.Header.Get("X-Client-Version")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := newClient(clientConfig{
		Endpoint: server.URL,
		Version:  "0.9.0",
	})

	_, err := c.Submit(context.Background(), []FrictionEvent{}, nil)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	if receivedVersion != "0.9.0" {
		t.Errorf("X-Client-Version = %q, want %q", receivedVersion, "0.9.0")
	}
}

func TestClient_Submit_SendsCatalogVersionHeader(t *testing.T) {
	var receivedCatalogVersion string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCatalogVersion = r.Header.Get("X-Catalog-Version")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := newClient(clientConfig{
		Endpoint: server.URL,
		Version:  "0.9.0",
	})

	opts := &submitOptions{CatalogVersion: "v2026-01-17-001"}
	_, err := c.Submit(context.Background(), []FrictionEvent{}, opts)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	if receivedCatalogVersion != "v2026-01-17-001" {
		t.Errorf("X-Catalog-Version = %q, want %q", receivedCatalogVersion, "v2026-01-17-001")
	}
}

func TestClient_Submit_ParsesCatalogFromResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := FrictionResponse{
			Accepted: 1,
			Catalog: &CatalogData{
				Version: "v2026-01-17-002",
				Tokens: []TokenMapping{
					{Pattern: "stauts", Target: "status", Kind: "unknown-command"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newClient(clientConfig{
		Endpoint: server.URL,
		Version:  "0.9.0",
	})

	resp, err := c.Submit(context.Background(), []FrictionEvent{
		{Kind: "unknown-command", Input: "mycli stauts"},
	}, nil)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	if resp.Catalog == nil {
		t.Fatal("Catalog should be parsed from response body")
	}
	if resp.Catalog.Version != "v2026-01-17-002" {
		t.Errorf("Catalog.Version = %q, want %q", resp.Catalog.Version, "v2026-01-17-002")
	}
	if len(resp.Catalog.Tokens) != 1 {
		t.Fatalf("Catalog.Tokens length = %d, want 1", len(resp.Catalog.Tokens))
	}
}

func TestClient_Submit_NoCatalogOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := FrictionResponse{
			Accepted: 0,
			Catalog: &CatalogData{
				Version: "v-should-not-parse",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newClient(clientConfig{
		Endpoint: server.URL,
		Version:  "0.9.0",
	})

	resp, err := c.Submit(context.Background(), []FrictionEvent{
		{Kind: "unknown-command", Input: "test"},
	}, nil)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	if resp.Catalog != nil {
		t.Errorf("Catalog should be nil on non-2xx response, got version %q", resp.Catalog.Version)
	}
}

func TestMaxEventsPerRequest(t *testing.T) {
	if maxEventsPerRequest != 100 {
		t.Errorf("maxEventsPerRequest = %d, want 100", maxEventsPerRequest)
	}
}

func TestClient_Submit_WithRequestDecorator(t *testing.T) {
	var receivedUA string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := newClient(clientConfig{
		Endpoint: server.URL,
		Version:  "0.5.0",
		RequestDecorator: func(req *http.Request) {
			req.Header.Set("User-Agent", "testcli/0.5.0")
		},
	})

	_, err := c.Submit(context.Background(), []FrictionEvent{}, nil)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	if receivedUA != "testcli/0.5.0" {
		t.Errorf("User-Agent = %q, want %q", receivedUA, "testcli/0.5.0")
	}
}
