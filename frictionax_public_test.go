package frictionax

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Option tests ---

func TestWithCatalog(t *testing.T) {
	cfg := &frictionConfig{}
	WithCatalog("mycli")(cfg)
	if cfg.cliName != "mycli" {
		t.Errorf("cliName = %q, want %q", cfg.cliName, "mycli")
	}
}

func TestWithTelemetry(t *testing.T) {
	cfg := &frictionConfig{}
	WithTelemetry("https://api.example.com", "1.2.3")(cfg)
	if cfg.endpoint != "https://api.example.com" {
		t.Errorf("endpoint = %q, want %q", cfg.endpoint, "https://api.example.com")
	}
	if cfg.version != "1.2.3" {
		t.Errorf("version = %q, want %q", cfg.version, "1.2.3")
	}
}

func TestWithAuth(t *testing.T) {
	cfg := &frictionConfig{}
	fn := func() string { return "my-token" }
	WithAuth(fn)(cfg)
	if cfg.authFunc == nil {
		t.Fatal("authFunc should not be nil")
	}
	if got := cfg.authFunc(); got != "my-token" {
		t.Errorf("authFunc() = %q, want %q", got, "my-token")
	}
}

func TestWithRedactor(t *testing.T) {
	cfg := &frictionConfig{}
	r := &testRedactor{suffix: "-redacted"}
	WithRedactor(r)(cfg)
	if cfg.redactor == nil {
		t.Fatal("redactor should not be nil")
	}
	if got := cfg.redactor.Redact("secret"); got != "secret-redacted" {
		t.Errorf("Redact() = %q, want %q", got, "secret-redacted")
	}
}

func TestWithActorDetector(t *testing.T) {
	cfg := &frictionConfig{}
	d := &mockActorDetector{actor: ActorAgent, agentType: "test-agent"}
	WithActorDetector(d)(cfg)
	if cfg.actorDetector == nil {
		t.Fatal("actorDetector should not be nil")
	}
	actor, agentType := cfg.actorDetector.DetectActor()
	if actor != ActorAgent {
		t.Errorf("actor = %q, want %q", actor, ActorAgent)
	}
	if agentType != "test-agent" {
		t.Errorf("agentType = %q, want %q", agentType, "test-agent")
	}
}

func TestWithLogger(t *testing.T) {
	cfg := &frictionConfig{}
	logger := slog.Default()
	WithLogger(logger)(cfg)
	if cfg.logger != logger {
		t.Error("logger should match the provided logger")
	}
}

func TestWithCachePath(t *testing.T) {
	cfg := &frictionConfig{}
	WithCachePath("/tmp/test-cache")(cfg)
	if cfg.cachePath != "/tmp/test-cache" {
		t.Errorf("cachePath = %q, want %q", cfg.cachePath, "/tmp/test-cache")
	}
}

func TestWithRequestDecorator(t *testing.T) {
	cfg := &frictionConfig{}
	called := false
	fn := func(r *http.Request) { called = true }
	WithRequestDecorator(fn)(cfg)
	if cfg.requestDecorator == nil {
		t.Fatal("requestDecorator should not be nil")
	}
	cfg.requestDecorator(&http.Request{})
	if !called {
		t.Error("requestDecorator should have been called")
	}
}

func TestWithIsEnabled(t *testing.T) {
	cfg := &frictionConfig{}
	WithIsEnabled(func() bool { return false })(cfg)
	if cfg.isEnabled == nil {
		t.Fatal("isEnabled should not be nil")
	}
	if cfg.isEnabled() {
		t.Error("isEnabled() should return false")
	}
}

// --- New() tests ---

func TestNew_Defaults(t *testing.T) {
	adapter := &mockCLIAdapter{}
	f := New(adapter)

	if f == nil {
		t.Fatal("Friction should not be nil")
	}
	if f.adapter != adapter {
		t.Error("adapter should match")
	}
	if f.engine == nil {
		t.Error("engine should be initialized")
	}
	if f.collector != nil {
		t.Error("collector should be nil without telemetry")
	}
	if f.catalog != nil {
		t.Error("catalog should be nil without WithCatalog")
	}
	if f.logger == nil {
		t.Error("logger should default to slog.Default()")
	}
	// default redactor should be noOpRedactor
	if got := f.redactor.Redact("test"); got != "test" {
		t.Errorf("default redactor should be no-op, got %q", got)
	}
}

func TestNew_WithCatalog(t *testing.T) {
	adapter := &mockCLIAdapter{}
	f := New(adapter, WithCatalog("testcli"))

	if f.catalog == nil {
		t.Fatal("catalog should be initialized when WithCatalog is used")
	}
}

func TestNew_WithTelemetry(t *testing.T) {
	// stand up a fake endpoint so the collector can be created
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"accepted":0}`)
	}))
	defer srv.Close()

	adapter := &mockCLIAdapter{}
	f := New(adapter, WithTelemetry(srv.URL, "0.1.0"))
	defer f.Close()

	if f.collector == nil {
		t.Fatal("collector should be initialized when WithTelemetry is used")
	}
}

func TestNew_WithCustomRedactor(t *testing.T) {
	r := &testRedactor{suffix: "-REDACTED"}
	adapter := &mockCLIAdapter{}
	f := New(adapter, WithRedactor(r))

	if got := f.redactor.Redact("secret"); got != "secret-REDACTED" {
		t.Errorf("custom redactor not applied, got %q", got)
	}
}

func TestNew_WithCustomActorDetector(t *testing.T) {
	d := &mockActorDetector{actor: ActorAgent, agentType: "custom-agent"}
	adapter := &mockCLIAdapter{}
	f := New(adapter, WithActorDetector(d))

	actor, agentType := f.actorDetector.DetectActor()
	if actor != ActorAgent || agentType != "custom-agent" {
		t.Errorf("custom actor detector not applied: actor=%q agentType=%q", actor, agentType)
	}
}

func TestNew_MultipleOptions(t *testing.T) {
	logger := slog.Default()
	r := &testRedactor{suffix: "-X"}
	d := &mockActorDetector{actor: ActorHuman}

	adapter := &mockCLIAdapter{}
	f := New(adapter,
		WithCatalog("multicli"),
		WithLogger(logger),
		WithRedactor(r),
		WithActorDetector(d),
		WithCachePath("/tmp/cache"),
	)

	if f.catalog == nil {
		t.Error("catalog should be set")
	}
	if f.logger != logger {
		t.Error("logger should match")
	}
	if got := f.redactor.Redact("x"); got != "x-X" {
		t.Errorf("redactor not applied, got %q", got)
	}
}

// --- Handle() tests ---

func TestHandle_NilParsedError(t *testing.T) {
	adapter := &mockCLIAdapter{parsedError: nil}
	f := New(adapter)

	result := f.Handle([]string{"mycli", "bad"}, errors.New("some error"))
	if result != nil {
		t.Error("Handle should return nil when ParseError returns nil")
	}
}

func TestHandle_UnknownCommand(t *testing.T) {
	adapter := &mockCLIAdapter{
		parsedError: &ParsedError{
			Kind:       FailureUnknownCommand,
			BadToken:   "statu",
			Command:    "",
			RawMessage: "unknown command statu",
		},
		commandNames: []string{"status", "login", "logout"},
	}
	f := New(adapter)
	result := f.Handle([]string{"mycli", "statu"}, errors.New("unknown command"))

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Event == nil {
		t.Fatal("event should not be nil")
	}
	if result.Event.Kind != FailureUnknownCommand {
		t.Errorf("kind = %q, want %q", result.Event.Kind, FailureUnknownCommand)
	}
	if result.Suggestion == nil {
		t.Fatal("suggestion should not be nil for close match")
	}
	if result.Suggestion.Corrected != "status" {
		t.Errorf("corrected = %q, want %q", result.Suggestion.Corrected, "status")
	}
}

func TestHandle_UnknownFlag(t *testing.T) {
	adapter := &mockCLIAdapter{
		parsedError: &ParsedError{
			Kind:       FailureUnknownFlag,
			BadToken:   "--verbos",
			Command:    "agent",
			RawMessage: "unknown flag --verbos",
		},
		flagNames: map[string][]string{
			"agent": {"--verbose", "--help", "--version"},
		},
	}
	f := New(adapter)
	result := f.Handle([]string{"mycli", "agent", "--verbos"}, errors.New("unknown flag"))

	if result == nil || result.Suggestion == nil {
		t.Fatal("should suggest correction for close flag match")
	}
	if result.Suggestion.Corrected != "--verbose" {
		t.Errorf("corrected = %q, want %q", result.Suggestion.Corrected, "--verbose")
	}
}

func TestHandle_NoSuggestionForDistantMatch(t *testing.T) {
	adapter := &mockCLIAdapter{
		parsedError: &ParsedError{
			Kind:       FailureUnknownCommand,
			BadToken:   "zzzzzzzzz",
			Command:    "",
			RawMessage: "unknown command",
		},
		commandNames: []string{"status", "login"},
	}
	f := New(adapter)
	result := f.Handle([]string{"mycli", "zzzzzzzzz"}, errors.New("unknown command"))

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Suggestion != nil {
		t.Errorf("should not suggest for distant match, got %+v", result.Suggestion)
	}
}

func TestHandle_EventFieldsPopulated(t *testing.T) {
	adapter := &mockCLIAdapter{
		parsedError: &ParsedError{
			Kind:       FailureUnknownCommand,
			BadToken:   "statu",
			Command:    "root",
			Subcommand: "sub",
			RawMessage: "error message here",
		},
		commandNames: []string{"status"},
	}
	f := New(adapter)
	result := f.Handle([]string{"mycli", "statu"}, errors.New("error"))

	event := result.Event
	if event.Timestamp == "" {
		t.Error("timestamp should be set")
	}
	if event.Command != "root" {
		t.Errorf("command = %q, want %q", event.Command, "root")
	}
	if event.Subcommand != "sub" {
		t.Errorf("subcommand = %q, want %q", event.Subcommand, "sub")
	}
	if event.Input == "" {
		t.Error("input should not be empty")
	}
	if event.ErrorMsg == "" {
		t.Error("error_msg should not be empty")
	}
	if event.Actor == "" {
		t.Error("actor should be populated")
	}
	if event.PathBucket == "" {
		t.Error("path_bucket should be populated")
	}
}

func TestHandle_WithCatalogAutoExecute(t *testing.T) {
	adapter := &mockCLIAdapter{
		parsedError: &ParsedError{
			Kind:       FailureUnknownFlag,
			BadToken:   "--every",
			Command:    "daemons",
			RawMessage: "unknown flag --every",
		},
		commandNames: []string{"daemons"},
	}

	f := New(adapter, WithCatalog("mycli"))
	// load catalog with an auto-execute mapping
	err := f.UpdateCatalog(CatalogData{
		Version: "v1",
		Commands: []CommandMapping{
			{
				Pattern:     "daemons list --every",
				Target:      "daemons show --all",
				AutoExecute: true,
				Confidence:  0.95,
				Description: "use show --all instead",
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateCatalog error: %v", err)
	}

	result := f.Handle([]string{"mycli", "daemons", "list", "--every"}, errors.New("unknown flag"))
	if result == nil || result.Suggestion == nil {
		t.Fatal("should return suggestion from catalog")
	}
	if !result.AutoExecute {
		t.Error("should auto-execute for high-confidence catalog match")
	}
	if len(result.CorrectedArgs) == 0 {
		t.Error("correctedArgs should be populated for auto-execute")
	}
}

func TestHandle_MissingRequiredDefaultsBranch(t *testing.T) {
	// FailureMissingRequired hits the default branch in the switch
	adapter := &mockCLIAdapter{
		parsedError: &ParsedError{
			Kind:       FailureMissingRequired,
			BadToken:   "",
			Command:    "agent",
			Subcommand: "prime",
			RawMessage: "missing required argument",
		},
		commandNames: []string{"agent", "login"},
	}
	f := New(adapter)
	result := f.Handle([]string{"mycli", "agent", "prime"}, errors.New("missing"))

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Event.Kind != FailureMissingRequired {
		t.Errorf("kind = %q, want %q", result.Event.Kind, FailureMissingRequired)
	}
}

// --- Record() tests ---

func TestRecord_NoCollector(t *testing.T) {
	f := New(&mockCLIAdapter{})
	// should not panic
	f.Record(FrictionEvent{Kind: FailureUnknownCommand})
}

func TestRecord_WithCollector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"accepted":1}`)
	}))
	defer srv.Close()

	f := New(&mockCLIAdapter{}, WithTelemetry(srv.URL, "0.1.0"))
	defer f.Close()

	// should not panic, event gets buffered
	f.Record(FrictionEvent{Kind: FailureUnknownCommand, Input: "test"})
}

// --- Close() tests ---

func TestClose_NoCollector(t *testing.T) {
	f := New(&mockCLIAdapter{})
	// should not panic
	f.Close()
}

func TestClose_WithCollector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"accepted":0}`)
	}))
	defer srv.Close()

	f := New(&mockCLIAdapter{}, WithTelemetry(srv.URL, "0.1.0"))
	// calling close should not panic or hang
	f.Close()
}

func TestClose_Idempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"accepted":0}`)
	}))
	defer srv.Close()

	f := New(&mockCLIAdapter{}, WithTelemetry(srv.URL, "0.1.0"))
	f.Close()
	f.Close() // second call should not panic
}

// --- UpdateCatalog() tests ---

func TestUpdateCatalog_NoCatalog(t *testing.T) {
	f := New(&mockCLIAdapter{})
	err := f.UpdateCatalog(CatalogData{Version: "v1"})
	if err != nil {
		t.Errorf("UpdateCatalog should be no-op without catalog, got err: %v", err)
	}
}

func TestUpdateCatalog_WithCatalog(t *testing.T) {
	f := New(&mockCLIAdapter{}, WithCatalog("testcli"))
	data := CatalogData{
		Version: "v2",
		Commands: []CommandMapping{
			{Pattern: "old-cmd", Target: "new-cmd", Confidence: 0.9},
		},
		Tokens: []TokenMapping{
			{Pattern: "depliy", Target: "deploy", Kind: FailureUnknownCommand, Confidence: 0.9},
		},
	}
	err := f.UpdateCatalog(data)
	if err != nil {
		t.Fatalf("UpdateCatalog error: %v", err)
	}
}

// --- Stats() tests ---

func TestStats_NoCollector(t *testing.T) {
	f := New(&mockCLIAdapter{})
	stats := f.Stats()

	if stats.Enabled {
		t.Error("Enabled should be false without collector")
	}
	if stats.BufferCount != 0 {
		t.Errorf("BufferCount = %d, want 0", stats.BufferCount)
	}
	if stats.BufferSize != 0 {
		t.Errorf("BufferSize = %d, want 0", stats.BufferSize)
	}
	if stats.SampleRate != 0 {
		t.Errorf("SampleRate = %f, want 0", stats.SampleRate)
	}
}

func TestStats_WithCollector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"accepted":0}`)
	}))
	defer srv.Close()

	f := New(&mockCLIAdapter{}, WithTelemetry(srv.URL, "0.1.0"))
	defer f.Close()

	stats := f.Stats()
	if !stats.Enabled {
		t.Error("Enabled should be true with collector")
	}
	if stats.BufferSize <= 0 {
		t.Errorf("BufferSize should be > 0, got %d", stats.BufferSize)
	}
}

// --- parseArgs tests ---

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "empty string", input: "", want: nil},
		{name: "single word", input: "status", want: []string{"status"}},
		{name: "multiple words", input: "agent prime --verbose", want: []string{"agent", "prime", "--verbose"}},
		{name: "extra whitespace", input: "  agent   prime  ", want: []string{"agent", "prime"}},
		{name: "tabs and spaces", input: "\tagent\t\tprime", want: []string{"agent", "prime"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseArgs(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseArgs(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseArgs(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// --- detectPathBucket tests ---

func TestDetectPathBucket(t *testing.T) {
	// we're running inside a git repo, so should get "repo"
	bucket := detectPathBucket()
	validBuckets := map[string]bool{
		pathBucketHome: true,
		pathBucketRepo: true,
		pathBucketOther: true,
	}
	if !validBuckets[bucket] {
		t.Errorf("detectPathBucket() = %q, not a valid bucket", bucket)
	}
}

// --- Stats JSON marshaling ---

func TestStats_JSONSerialization(t *testing.T) {
	s := Stats{
		Enabled:        true,
		BufferCount:    5,
		BufferSize:     100,
		SampleRate:     1.0,
		CatalogVersion: "v3",
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded["enabled"] != true {
		t.Error("enabled should be true")
	}
	if decoded["buffer_count"] != float64(5) {
		t.Errorf("buffer_count = %v, want 5", decoded["buffer_count"])
	}
	if decoded["catalog_version"] != "v3" {
		t.Errorf("catalog_version = %v, want v3", decoded["catalog_version"])
	}
}

// --- Handle with custom redactor ---

func TestHandle_WithRedactor(t *testing.T) {
	r := &testRedactor{suffix: ""}
	r.replaceAll = true
	adapter := &mockCLIAdapter{
		parsedError: &ParsedError{
			Kind:       FailureUnknownCommand,
			BadToken:   "statu",
			Command:    "",
			RawMessage: "unknown command statu with secret_key=ABCDEF",
		},
		commandNames: []string{"status"},
	}
	f := New(adapter, WithRedactor(r))
	result := f.Handle([]string{"mycli", "statu"}, errors.New("err"))

	if result == nil || result.Event == nil {
		t.Fatal("result should not be nil")
	}
	// the redactor was called on input and error
	if !strings.Contains(result.Event.Input, "[REDACTED]") {
		t.Errorf("input should be redacted, got %q", result.Event.Input)
	}
}

// --- Handle with custom actor detector via New ---

func TestHandle_ActorDetectorFromOption(t *testing.T) {
	adapter := &mockCLIAdapter{
		parsedError: &ParsedError{
			Kind:       FailureUnknownCommand,
			BadToken:   "statu",
			Command:    "",
			RawMessage: "unknown command",
		},
		commandNames: []string{"status"},
	}
	d := &mockActorDetector{actor: ActorAgent, agentType: "copilot"}
	f := New(adapter, WithActorDetector(d))
	result := f.Handle([]string{"mycli", "statu"}, errors.New("err"))

	if result.Event.Actor != string(ActorAgent) {
		t.Errorf("actor = %q, want %q", result.Event.Actor, ActorAgent)
	}
	if result.Event.AgentType != "copilot" {
		t.Errorf("agentType = %q, want %q", result.Event.AgentType, "copilot")
	}
}

// --- test helpers ---

type testRedactor struct {
	suffix     string
	replaceAll bool
}

func (r *testRedactor) Redact(input string) string {
	if r.replaceAll {
		return "[REDACTED]"
	}
	return input + r.suffix
}
