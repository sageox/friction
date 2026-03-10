package frictionax

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// mockCLIAdapter implements CLIAdapter for testing.
type mockCLIAdapter struct {
	commandNames []string
	flagNames    map[string][]string
	parsedError  *ParsedError
}

func (m *mockCLIAdapter) CommandNames() []string {
	return m.commandNames
}

func (m *mockCLIAdapter) FlagNames(command string) []string {
	if m.flagNames == nil {
		return nil
	}
	return m.flagNames[command]
}

func (m *mockCLIAdapter) ParseError(err error) *ParsedError {
	return m.parsedError
}

// mockActorDetector implements ActorDetector for testing.
type mockActorDetector struct {
	actor     Actor
	agentType string
}

func (m *mockActorDetector) DetectActor() (Actor, string) {
	return m.actor, m.agentType
}

func TestNewHandler(t *testing.T) {
	tests := []struct {
		name    string
		adapter CLIAdapter
		catalog catalog
	}{
		{
			name:    "creates handler with adapter and catalog",
			adapter: &mockCLIAdapter{},
			catalog: newMockCatalog("testcli"),
		},
		{
			name:    "creates handler with nil catalog",
			adapter: &mockCLIAdapter{},
			catalog: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHandler(tt.adapter, tt.catalog, nil, nil)
			if h == nil {
				t.Fatal("handler should not be nil")
			}
			if h.engine == nil {
				t.Fatal("engine should not be nil")
			}
		})
	}
}

func TestHandler_HandleWithAutoExecute(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		err               error
		adapter           *mockCLIAdapter
		catalog           *mockCatalog
		wantNil           bool
		wantKind          FailureKind
		wantSuggestionSet bool
	}{
		{
			name: "returns nil for non-parseable error",
			args: []string{"mycli", "invalid"},
			err:  errors.New("some error"),
			adapter: &mockCLIAdapter{
				parsedError: nil,
			},
			catalog: newMockCatalog("testcli"),
			wantNil: true,
		},
		{
			name: "handles unknown command error",
			args: []string{"mycli", "statu"},
			err:  errors.New("unknown command statu"),
			adapter: &mockCLIAdapter{
				parsedError: &ParsedError{
					Kind:       FailureUnknownCommand,
					BadToken:   "statu",
					Command:    "",
					RawMessage: "unknown command statu",
				},
				commandNames: []string{"status", "login", "logout", "agent"},
			},
			catalog:           newMockCatalog("testcli"),
			wantNil:           false,
			wantKind:          FailureUnknownCommand,
			wantSuggestionSet: true,
		},
		{
			name: "handles unknown flag error",
			args: []string{"mycli", "agent", "--verbos"},
			err:  errors.New("unknown flag --verbos"),
			adapter: &mockCLIAdapter{
				parsedError: &ParsedError{
					Kind:       FailureUnknownFlag,
					BadToken:   "--verbos",
					Command:    "agent",
					RawMessage: "unknown flag --verbos",
				},
				flagNames: map[string][]string{
					"agent": {"--verbose", "--help", "--version"},
				},
			},
			catalog:           newMockCatalog("testcli"),
			wantNil:           false,
			wantKind:          FailureUnknownFlag,
			wantSuggestionSet: true,
		},
		{
			name: "uses catalog command mapping when available",
			args: []string{"mycli", "daemons", "list", "--every"},
			err:  errors.New("unknown flag"),
			adapter: &mockCLIAdapter{
				parsedError: &ParsedError{
					Kind:       FailureUnknownFlag,
					BadToken:   "--every",
					Command:    "daemons",
					RawMessage: "unknown flag --every",
				},
				commandNames: []string{"daemons"},
			},
			catalog: func() *mockCatalog {
				c := newMockCatalog("mycli")
				c.addCommand("daemons list --every", "daemons show --all", 0.95, "use 'show --all' instead")
				return c
			}(),
			wantNil:           false,
			wantKind:          FailureUnknownFlag,
			wantSuggestionSet: true,
		},
		{
			name: "handles missing required argument",
			args: []string{"mycli", "agent", "prime"},
			err:  errors.New("missing required argument"),
			adapter: &mockCLIAdapter{
				parsedError: &ParsedError{
					Kind:       FailureMissingRequired,
					BadToken:   "",
					Command:    "agent",
					Subcommand: "prime",
					RawMessage: "missing required argument",
				},
				commandNames: []string{"agent"},
			},
			catalog:           newMockCatalog("testcli"),
			wantNil:           false,
			wantKind:          FailureMissingRequired,
			wantSuggestionSet: false,
		},
		{
			name: "uses token mapping from catalog",
			args: []string{"mycli", "depliy"},
			err:  errors.New("unknown command depliy"),
			adapter: &mockCLIAdapter{
				parsedError: &ParsedError{
					Kind:       FailureUnknownCommand,
					BadToken:   "depliy",
					Command:    "",
					RawMessage: "unknown command depliy",
				},
				commandNames: []string{"deploy", "status"},
			},
			catalog: func() *mockCatalog {
				c := newMockCatalog("mycli")
				c.addToken("depliy", "deploy", FailureUnknownCommand, 0.9)
				return c
			}(),
			wantNil:           false,
			wantKind:          FailureUnknownCommand,
			wantSuggestionSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHandler(tt.adapter, tt.catalog, nil, nil)
			result := h.HandleWithAutoExecute(tt.args, tt.err)

			if tt.wantNil {
				if result != nil {
					t.Fatal("result should be nil")
				}
				return
			}

			if result == nil {
				t.Fatal("result should not be nil")
			}
			if result.Event == nil {
				t.Fatal("event should not be nil")
			}

			if result.Event.Kind != tt.wantKind {
				t.Errorf("kind = %q, want %q", result.Event.Kind, tt.wantKind)
			}

			if tt.wantSuggestionSet {
				if result.Suggestion == nil {
					t.Error("suggestion should not be nil")
				}
			}

			if result.Event.Timestamp == "" {
				t.Error("timestamp should be set")
			}
			if result.Event.Input == "" {
				t.Error("input should be set")
			}
		})
	}
}

func TestDetectActor(t *testing.T) {
	tests := []struct {
		name          string
		setupEnv      func()
		teardownEnv   func()
		wantActor     Actor
		wantAgentType string
	}{
		{
			name: "detects CI environment as agent",
			setupEnv: func() {
				os.Setenv("CI", "true")
			},
			teardownEnv: func() {
				os.Unsetenv("CI")
			},
			wantActor:     ActorAgent,
			wantAgentType: "ci",
		},
		{
			name: "detects human when no agent or CI env",
			setupEnv: func() {
				os.Unsetenv("CI")
				os.Unsetenv("CLAUDE_CODE")
				os.Unsetenv("AGENT_ENV")
			},
			teardownEnv: func() {},
			wantActor:   ActorHuman,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()
			defer tt.teardownEnv()

			detector := envActorDetector{}
			actor, agentType := detector.DetectActor()

			if actor != tt.wantActor {
				t.Errorf("actor = %q, want %q", actor, tt.wantActor)
			}
			if tt.wantAgentType != "" && agentType != tt.wantAgentType {
				t.Errorf("agent type = %q, want %q", agentType, tt.wantAgentType)
			}
		})
	}
}

func TestFormatSuggestion(t *testing.T) {
	tests := []struct {
		name       string
		suggestion *Suggestion
		jsonMode   bool
		want       string
		wantJSON   map[string]any
	}{
		{
			name:       "nil suggestion returns empty string",
			suggestion: nil,
			jsonMode:   false,
			want:       "",
		},
		{
			name:       "nil suggestion returns empty string in json mode",
			suggestion: nil,
			jsonMode:   true,
			want:       "",
		},
		{
			name: "human format without description",
			suggestion: &Suggestion{
				Type:       SuggestionLevenshtein,
				Original:   "statu",
				Corrected:  "status",
				Confidence: 0.8,
			},
			jsonMode: false,
			want:     "Did you mean this?\n    status",
		},
		{
			name: "human format with description",
			suggestion: &Suggestion{
				Type:        SuggestionCommandRemap,
				Original:    "daemons list --every",
				Corrected:   "daemons show --all",
				Confidence:  0.95,
				Description: "use show --all instead",
			},
			jsonMode: false,
			want:     "Did you mean this?\n    daemons show --all",
		},
		{
			name: "json format without description",
			suggestion: &Suggestion{
				Type:       SuggestionLevenshtein,
				Original:   "statu",
				Corrected:  "status",
				Confidence: 0.8,
			},
			jsonMode: true,
			wantJSON: map[string]any{
				"type":       "levenshtein",
				"suggestion": "status",
				"confidence": 0.8,
			},
		},
		{
			name: "json format with description",
			suggestion: &Suggestion{
				Type:        SuggestionCommandRemap,
				Original:    "daemons list --every",
				Corrected:   "daemons show --all",
				Confidence:  0.95,
				Description: "use show --all instead",
			},
			jsonMode: true,
			wantJSON: map[string]any{
				"type":        "command-remap",
				"suggestion":  "daemons show --all",
				"confidence":  0.95,
				"description": "use show --all instead",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSuggestion(tt.suggestion, tt.jsonMode)

			if tt.jsonMode && tt.wantJSON != nil {
				var got map[string]any
				if err := json.Unmarshal([]byte(result), &got); err != nil {
					t.Fatalf("result should be valid JSON: %v", err)
				}
				for k, v := range tt.wantJSON {
					if got[k] != v {
						t.Errorf("JSON key %q = %v, want %v", k, got[k], v)
					}
				}
			} else {
				if result != tt.want {
					t.Errorf("format = %q, want %q", result, tt.want)
				}
			}
		})
	}
}

func TestHandler_HandleWithAutoExecute_BuildsFrictionEventCorrectly(t *testing.T) {
	adapter := &mockCLIAdapter{
		parsedError: &ParsedError{
			Kind:       FailureUnknownCommand,
			BadToken:   "statu",
			Command:    "mycli",
			Subcommand: "",
			RawMessage: "unknown command 'statu' for 'mycli'",
		},
		commandNames: []string{"status", "login", "agent"},
	}
	cat := newMockCatalog("testcli")

	h := newHandler(adapter, cat, nil, nil)
	result := h.HandleWithAutoExecute([]string{"mycli", "statu"}, errors.New("unknown command"))

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Event == nil {
		t.Fatal("event should not be nil")
	}

	event := result.Event

	if event.Timestamp == "" {
		t.Error("timestamp should be set")
	}
	if event.Kind != FailureUnknownCommand {
		t.Errorf("kind = %q, want %q", event.Kind, FailureUnknownCommand)
	}
	if event.Actor == "" {
		t.Error("actor should be set")
	}
	if !contains(event.Input, "mycli statu") {
		t.Errorf("input %q should contain 'mycli statu'", event.Input)
	}
	if !contains(event.ErrorMsg, "unknown command") {
		t.Errorf("error_msg %q should contain 'unknown command'", event.ErrorMsg)
	}

	if result.Suggestion == nil {
		t.Fatal("suggestion should not be nil")
	}
	if result.Suggestion.Corrected != "status" {
		t.Errorf("suggestion = %q, want 'status'", result.Suggestion.Corrected)
	}
}

func TestHandler_HandleWithAutoExecute_NoSuggestionWhenNoMatch(t *testing.T) {
	adapter := &mockCLIAdapter{
		parsedError: &ParsedError{
			Kind:       FailureUnknownCommand,
			BadToken:   "xyzabc",
			Command:    "",
			RawMessage: "unknown command xyzabc",
		},
		commandNames: []string{"status", "login", "agent"},
	}
	cat := newMockCatalog("testcli")

	h := newHandler(adapter, cat, nil, nil)
	result := h.HandleWithAutoExecute([]string{"mycli", "xyzabc"}, errors.New("unknown command"))

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Event == nil {
		t.Fatal("event should not be nil")
	}
	if result.Suggestion != nil {
		t.Errorf("suggestion should be nil for unmatched input, got %+v", result.Suggestion)
	}
}

func TestHandler_HandleWithAutoExecute_ActorAndAgentTypePopulated(t *testing.T) {
	originalCI := os.Getenv("CI")
	defer func() {
		if originalCI == "" {
			os.Unsetenv("CI")
		} else {
			os.Setenv("CI", originalCI)
		}
	}()

	os.Setenv("CI", "true")

	adapter := &mockCLIAdapter{
		parsedError: &ParsedError{
			Kind:       FailureUnknownCommand,
			BadToken:   "test",
			Command:    "",
			RawMessage: "unknown command test",
		},
		commandNames: []string{"status"},
	}
	cat := newMockCatalog("testcli")

	h := newHandler(adapter, cat, nil, nil)
	result := h.HandleWithAutoExecute([]string{"mycli", "test"}, errors.New("unknown command"))

	if result == nil || result.Event == nil {
		t.Fatal("result and event should not be nil")
	}

	if result.Event.Actor != string(ActorAgent) {
		t.Errorf("actor = %q, want %q", result.Event.Actor, ActorAgent)
	}
	if result.Event.AgentType != "ci" {
		t.Errorf("agent_type = %q, want 'ci'", result.Event.AgentType)
	}
}

func TestHandler_HandleWithAutoExecute_EmptyArgs(t *testing.T) {
	adapter := &mockCLIAdapter{
		parsedError: &ParsedError{
			Kind:       FailureUnknownCommand,
			BadToken:   "",
			Command:    "",
			RawMessage: "no command specified",
		},
		commandNames: []string{"status", "login"},
	}
	cat := newMockCatalog("testcli")

	h := newHandler(adapter, cat, nil, nil)
	result := h.HandleWithAutoExecute([]string{}, errors.New("no command"))

	if result == nil {
		t.Fatal("result should not be nil even with empty args")
	}
	if result.Event == nil {
		t.Fatal("event should not be nil")
	}
	if result.Suggestion != nil {
		t.Error("suggestion should be nil with empty args")
	}
}

func TestHandler_HandleWithAutoExecute_SpecialCharactersInArgs(t *testing.T) {
	adapter := &mockCLIAdapter{
		parsedError: &ParsedError{
			Kind:       FailureUnknownCommand,
			BadToken:   "test--cmd",
			Command:    "",
			RawMessage: "unknown command test--cmd",
		},
		commandNames: []string{"test-cmd", "test"},
	}
	cat := newMockCatalog("testcli")

	h := newHandler(adapter, cat, nil, nil)
	result := h.HandleWithAutoExecute([]string{"mycli", "test--cmd", "--flag=value"}, errors.New("unknown"))

	if result == nil || result.Event == nil {
		t.Fatal("result and event should not be nil")
	}

	if !contains(result.Event.Input, "test--cmd") {
		t.Errorf("input should contain args with special chars")
	}
	if !contains(result.Event.Input, "--flag=value") {
		t.Errorf("input should contain flag with value")
	}
}

func TestNewHandler_EngineConfigured(t *testing.T) {
	adapter := &mockCLIAdapter{}
	cat := newMockCatalog("testcli")

	h := newHandler(adapter, cat, nil, nil)

	if h.engine == nil {
		t.Fatal("engine should be created")
	}
	if h.engine.levenshtein == nil {
		t.Fatal("levenshtein suggester should be created")
	}
}

func TestHandler_HandleWithAutoExecute_SuggestionPriority(t *testing.T) {
	adapter := &mockCLIAdapter{
		parsedError: &ParsedError{
			Kind:       FailureUnknownCommand,
			BadToken:   "statu",
			Command:    "",
			RawMessage: "unknown command statu",
		},
		commandNames: []string{"status"},
	}

	cat := func() *mockCatalog {
		c := newMockCatalog("mycli")
		c.addCommand("statu", "status --verbose", 0.95, "common pattern")
		return c
	}()

	h := newHandler(adapter, cat, nil, nil)
	result := h.HandleWithAutoExecute([]string{"mycli", "statu"}, errors.New("unknown command"))

	if result == nil || result.Suggestion == nil {
		t.Fatal("result and suggestion should not be nil")
	}

	if result.Suggestion.Type != SuggestionCommandRemap {
		t.Errorf("type = %q, want %q", result.Suggestion.Type, SuggestionCommandRemap)
	}
	if result.Suggestion.Corrected != "status --verbose" {
		t.Errorf("corrected = %q, want 'status --verbose'", result.Suggestion.Corrected)
	}
}

func TestHandler_WithCustomActorDetector(t *testing.T) {
	adapter := &mockCLIAdapter{
		parsedError: &ParsedError{
			Kind:       FailureUnknownCommand,
			BadToken:   "test",
			Command:    "",
			RawMessage: "unknown command test",
		},
		commandNames: []string{"status"},
	}

	detector := &mockActorDetector{
		actor:     ActorAgent,
		agentType: "claude-code",
	}

	h := newHandler(adapter, nil, detector, nil)
	result := h.HandleWithAutoExecute([]string{"mycli", "test"}, errors.New("unknown command"))

	if result == nil || result.Event == nil {
		t.Fatal("result and event should not be nil")
	}

	if result.Event.Actor != string(ActorAgent) {
		t.Errorf("actor = %q, want %q", result.Event.Actor, ActorAgent)
	}
	if result.Event.AgentType != "claude-code" {
		t.Errorf("agent_type = %q, want 'claude-code'", result.Event.AgentType)
	}
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsString(s, substr))
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
