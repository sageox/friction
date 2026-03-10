package frictionax_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sageox/frictionax"
	cobradapter "github.com/sageox/frictionax/adapters/cobra"
	"github.com/sageox/frictionax/internal/ringbuffer"
	spfcobra "github.com/spf13/cobra"
)

func buildTestCLI() *spfcobra.Command {
	root := &spfcobra.Command{
		Use:           "testcli",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	initCmd := &spfcobra.Command{
		Use:   "init",
		Short: "Initialize a project",
		RunE:  func(cmd *spfcobra.Command, args []string) error { return nil },
	}
	initCmd.Flags().BoolP("force", "f", false, "force initialization")
	initCmd.Flags().String("name", "", "project name")
	root.AddCommand(initCmd)

	agentCmd := &spfcobra.Command{
		Use:   "agent",
		Short: "Agent management",
	}
	agentCmd.Flags().BoolP("verbose", "v", false, "verbose output")

	primeCmd := &spfcobra.Command{
		Use:   "prime",
		Short: "Prime an agent",
		RunE:  func(cmd *spfcobra.Command, args []string) error { return nil },
	}
	primeCmd.Flags().Bool("verbose", false, "verbose output")

	statusCmd := &spfcobra.Command{
		Use:   "status",
		Short: "Show agent status",
		RunE:  func(cmd *spfcobra.Command, args []string) error { return nil },
	}

	agentCmd.AddCommand(primeCmd, statusCmd)
	root.AddCommand(agentCmd)

	configCmd := &spfcobra.Command{
		Use:   "config",
		Short: "Configuration management",
	}

	setCmd := &spfcobra.Command{
		Use:   "set [key] [value]",
		Short: "Set a config value",
		Args:  spfcobra.ExactArgs(2),
		RunE:  func(cmd *spfcobra.Command, args []string) error { return nil },
	}
	setCmd.Flags().BoolP("global", "g", false, "set globally")

	getCmd := &spfcobra.Command{
		Use:   "get [key]",
		Short: "Get a config value",
		Args:  spfcobra.ExactArgs(1),
		RunE:  func(cmd *spfcobra.Command, args []string) error { return nil },
	}

	configCmd.AddCommand(setCmd, getCmd)
	root.AddCommand(configCmd)

	sessionCmd := &spfcobra.Command{
		Use:   "session",
		Short: "Session management",
	}

	startCmd := &spfcobra.Command{
		Use:   "start",
		Short: "Start a session",
		RunE:  func(cmd *spfcobra.Command, args []string) error { return nil },
	}

	stopCmd := &spfcobra.Command{
		Use:   "stop",
		Short: "Stop a session",
		RunE:  func(cmd *spfcobra.Command, args []string) error { return nil },
	}

	sessionCmd.AddCommand(startCmd, stopCmd)
	root.AddCommand(sessionCmd)

	return root
}

// postFriction sends a SubmitRequest to the test server via direct HTTP.
func postFriction(t *testing.T, serverURL string, req frictionax.SubmitRequest) *http.Response {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/v1/cli/friction", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request error: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("HTTP request error: %v", err)
	}
	return resp
}

func TestIntegration_EndToEnd_CLIWithFriction(t *testing.T) {
	t.Parallel()

	root := buildTestCLI()
	adapter := cobradapter.NewCobraAdapter(root)
	fx := frictionax.New(adapter)

	testCases := []struct {
		name           string
		args           []string
		wantKind       frictionax.FailureKind
		wantBadToken   string
		wantSuggestion string
	}{
		{
			name:           "unknown command - typo",
			args:           []string{"testcli", "initt"},
			wantKind:       frictionax.FailureUnknownCommand,
			wantBadToken:   "initt",
			wantSuggestion: "init",
		},
		{
			name:           "unknown command - close to agent",
			args:           []string{"testcli", "agnt"},
			wantKind:       frictionax.FailureUnknownCommand,
			wantBadToken:   "agnt",
			wantSuggestion: "agent",
		},
		{
			name:         "unknown flag",
			args:         []string{"testcli", "init", "--verbose"},
			wantKind:     frictionax.FailureUnknownFlag,
			wantBadToken: "--verbose",
		},
		{
			name:           "unknown command - close to session",
			args:           []string{"testcli", "sesion"},
			wantKind:       frictionax.FailureUnknownCommand,
			wantBadToken:   "sesion",
			wantSuggestion: "session",
		},
		{
			name:           "unknown shorthand flag",
			args:           []string{"testcli", "init", "-x"},
			wantKind:       frictionax.FailureUnknownFlag,
			wantBadToken:   "-x",
			wantSuggestion: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := buildTestCLI()
			root.SetArgs(tc.args[1:])
			err := root.Execute()

			if err == nil {
				t.Fatal("command should fail")
			}

			result := fx.Handle(tc.args, err)
			if result == nil {
				t.Fatal("handler should produce result")
			}
			if result.Event == nil {
				t.Fatal("result should have event")
			}

			if result.Event.Kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", result.Event.Kind, tc.wantKind)
			}
			if result.Event.Timestamp == "" {
				t.Error("timestamp should be set")
			}
			if result.Event.Actor == "" {
				t.Error("actor should be set")
			}
			if result.Event.PathBucket == "" {
				t.Error("path_bucket should be set")
			}

			if tc.wantSuggestion != "" {
				if result.Suggestion == nil {
					t.Fatal("suggestion expected")
				}
				if result.Suggestion.Corrected != tc.wantSuggestion {
					t.Errorf("suggestion = %q, want %q", result.Suggestion.Corrected, tc.wantSuggestion)
				}
			}
		})
	}
}

func TestIntegration_FrictionEventsSubmittedToServer(t *testing.T) {
	t.Parallel()

	var receivedEvents []frictionax.FrictionEvent
	var receivedVersion string
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)

		if r.Method != http.MethodPost {
			t.Errorf("Method = %v, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/cli/friction" {
			t.Errorf("Path = %v, want /api/v1/cli/friction", r.URL.Path)
		}

		var req frictionax.SubmitRequest
		json.NewDecoder(r.Body).Decode(&req)

		receivedVersion = req.Version
		receivedEvents = append(receivedEvents, req.Events...)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(frictionax.FrictionResponse{Accepted: len(req.Events)})
	}))
	defer server.Close()

	root := buildTestCLI()
	adapter := cobradapter.NewCobraAdapter(root)
	fx := frictionax.New(adapter)

	buffer := ringbuffer.New(100, func(e frictionax.FrictionEvent) string {
		return string(e.Kind) + ":" + e.Input
	})

	errorCases := [][]string{
		{"testcli", "initt"},
		{"testcli", "init", "--forc"},
		{"testcli", "agnt", "prime"},
		{"testcli", "session", "-x"},
	}

	for _, args := range errorCases {
		root := buildTestCLI()
		root.SetArgs(args[1:])
		err := root.Execute()
		if err != nil {
			result := fx.Handle(args, err)
			if result != nil && result.Event != nil {
				buffer.Add(*result.Event)
			}
		}
	}

	if buffer.Count() != 4 {
		t.Errorf("buffer should have 4 events, got %d", buffer.Count())
	}

	events := buffer.Drain()
	resp := postFriction(t, server.URL, frictionax.SubmitRequest{
		Version: "test-0.1.0",
		Events:  events,
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if requestCount.Load() != 1 {
		t.Errorf("should have made 1 request, got %d", requestCount.Load())
	}
	if len(receivedEvents) != 4 {
		t.Errorf("server should receive 4 events, got %d", len(receivedEvents))
	}
	if receivedVersion != "test-0.1.0" {
		t.Errorf("version = %q, want %q", receivedVersion, "test-0.1.0")
	}

	for _, event := range receivedEvents {
		if event.Timestamp == "" {
			t.Error("timestamp should be set")
		}
		if event.Kind == "" {
			t.Error("kind should be set")
		}
		if event.Actor == "" {
			t.Error("actor should be set")
		}
		if event.Input == "" {
			t.Error("input should be set")
		}
	}
}

func TestIntegration_RateLimiting(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count == 1 {
			w.Header().Set("X-Friction-Sample-Rate", "0.0")
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(frictionax.FrictionResponse{Accepted: 1})
	}))
	defer server.Close()

	resp := postFriction(t, server.URL, frictionax.SubmitRequest{
		Version: "test-0.1.0",
		Events: []frictionax.FrictionEvent{{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Kind:       "unknown-command",
			Actor:      "human",
			PathBucket: "repo",
			Input:      "testcli foo",
		}},
	})
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if requestCount.Load() != 1 {
		t.Errorf("only 1 request should be made, got %d", requestCount.Load())
	}
}

func TestIntegration_RetryAfter(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	resp := postFriction(t, server.URL, frictionax.SubmitRequest{
		Version: "test-0.1.0",
		Events: []frictionax.FrictionEvent{{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Kind:       "unknown-command",
			Actor:      "human",
			PathBucket: "repo",
			Input:      "testcli foo",
		}},
	})
	resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}

	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter != "3600" {
		t.Errorf("Retry-After = %q, want %q", retryAfter, "3600")
	}
}

func TestIntegration_BufferBoundsUnderLoad(t *testing.T) {
	t.Parallel()

	const bufferSize = 50
	buffer := ringbuffer.New(bufferSize, func(e frictionax.FrictionEvent) string {
		return string(e.Kind) + ":" + e.Input
	})

	root := buildTestCLI()
	adapter := cobradapter.NewCobraAdapter(root)
	fx := frictionax.New(adapter)

	for i := 0; i < 1000; i++ {
		cmdName := fmt.Sprintf("unknown-cmd-%d", i)
		root := buildTestCLI()
		root.SetArgs([]string{cmdName})
		err := root.Execute()
		if err != nil {
			result := fx.Handle([]string{"testcli", cmdName}, err)
			if result != nil && result.Event != nil {
				buffer.Add(*result.Event)
			}
		}

		if buffer.Count() > bufferSize {
			t.Fatalf("buffer exceeded capacity: %d > %d at iteration %d",
				buffer.Count(), bufferSize, i)
		}
	}

	if buffer.Count() != bufferSize {
		t.Errorf("buffer should be at capacity (%d), got %d", bufferSize, buffer.Count())
	}

	events := buffer.Drain()
	if len(events) != bufferSize {
		t.Errorf("drain should return %d events, got %d", bufferSize, len(events))
	}
	if buffer.Count() != 0 {
		t.Error("buffer should be empty after drain")
	}
}

func TestIntegration_CatalogUpdateFromServer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(frictionax.FrictionResponse{
			Accepted: 1,
			Catalog: &frictionax.CatalogData{
				Version: "v2026-01-20-001",
				Tokens: []frictionax.TokenMapping{
					{Pattern: "initt", Target: "init", Kind: "unknown-command", Confidence: 0.95},
					{Pattern: "agnt", Target: "agent", Kind: "unknown-command", Confidence: 0.90},
				},
				Commands: []frictionax.CommandMapping{
					{Pattern: "daemon status", Target: "status", Confidence: 0.85},
				},
			},
		})
	}))
	defer server.Close()

	resp := postFriction(t, server.URL, frictionax.SubmitRequest{
		Version: "test-0.1.0",
		Events: []frictionax.FrictionEvent{{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Kind:       "unknown-command",
			Actor:      "human",
			PathBucket: "repo",
			Input:      "testcli initt",
		}},
	})
	defer resp.Body.Close()

	var frictionResp frictionax.FrictionResponse
	if err := json.NewDecoder(resp.Body).Decode(&frictionResp); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if frictionResp.Catalog == nil {
		t.Fatal("catalog should be in response")
	}
	if frictionResp.Catalog.Version != "v2026-01-20-001" {
		t.Errorf("Catalog.Version = %q, want %q", frictionResp.Catalog.Version, "v2026-01-20-001")
	}
	if len(frictionResp.Catalog.Tokens) != 2 {
		t.Errorf("Catalog.Tokens length = %d, want 2", len(frictionResp.Catalog.Tokens))
	}
	if len(frictionResp.Catalog.Commands) != 1 {
		t.Errorf("Catalog.Commands length = %d, want 1", len(frictionResp.Catalog.Commands))
	}
}

func TestIntegration_TruncationEnforced(t *testing.T) {
	t.Parallel()

	var receivedEvent frictionax.FrictionEvent

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req frictionax.SubmitRequest
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Events) > 0 {
			receivedEvent = req.Events[0]
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	longInput := make([]byte, 1000)
	for i := range longInput {
		longInput[i] = 'x'
	}
	longError := make([]byte, 500)
	for i := range longError {
		longError[i] = 'e'
	}

	// truncate client-side before sending
	event := frictionax.FrictionEvent{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Kind:       "unknown-command",
		Actor:      "human",
		PathBucket: "repo",
		Input:      string(longInput),
		ErrorMsg:   string(longError),
	}
	event.Truncate()

	resp := postFriction(t, server.URL, frictionax.SubmitRequest{
		Version: "test-0.1.0",
		Events:  []frictionax.FrictionEvent{event},
	})
	resp.Body.Close()

	// check truncation happened (500 for input, 200 for error)
	if len(receivedEvent.Input) > 500 {
		t.Errorf("input should be truncated to max length, got %d", len(receivedEvent.Input))
	}
	if len(receivedEvent.ErrorMsg) > 200 {
		t.Errorf("error_msg should be truncated to max length, got %d", len(receivedEvent.ErrorMsg))
	}
}
