package frictionax

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// FailureKind categorizes CLI usage failures for analytics and suggestion routing.
type FailureKind string

const (
	// FailureUnknownCommand indicates an unknown command was entered.
	FailureUnknownCommand FailureKind = "unknown-command"

	// FailureUnknownFlag indicates an unknown flag was provided.
	FailureUnknownFlag FailureKind = "unknown-flag"

	// FailureMissingRequired indicates a required argument or flag is missing.
	FailureMissingRequired FailureKind = "missing-required"

	// FailureInvalidArg indicates an argument has an invalid value.
	FailureInvalidArg FailureKind = "invalid-arg"

	// FailureParseError indicates a general CLI parsing failure.
	FailureParseError FailureKind = "parse-error"
)

// Actor identifies who initiated the command (human user or AI agent).
type Actor string

const (
	// ActorHuman indicates a human user typed the command.
	ActorHuman Actor = "human"

	// ActorAgent indicates an AI coding agent generated the command.
	ActorAgent Actor = "agent"

	// ActorUnknown indicates the actor could not be determined.
	ActorUnknown Actor = "unknown"
)

// SuggestionType indicates the source/method used to generate a suggestion.
type SuggestionType string

const (
	// SuggestionCommandRemap indicates a full command remap from the catalog.
	SuggestionCommandRemap SuggestionType = "command-remap"

	// SuggestionTokenFix indicates a single token correction from the catalog.
	SuggestionTokenFix SuggestionType = "token-fix"

	// SuggestionLevenshtein indicates an edit-distance based guess.
	SuggestionLevenshtein SuggestionType = "levenshtein"
)

// executeAction indicates what action to take with a suggestion (internal).
type executeAction string

const (
	actionSuggestOnly executeAction = "suggest"
	actionAutoExecute executeAction = "auto_execute"
)

// FrictionEvent captures a CLI usage failure for analytics.
// Events are privacy-preserving: inputs are redacted, errors are truncated.
//
// Field limits:
//   - Input: max 500 characters
//   - ErrorMsg: max 200 characters
//
// Use Truncate() to enforce these limits before submission.
type FrictionEvent struct {
	Timestamp  string      `json:"ts"`
	Kind       FailureKind `json:"kind"`
	Command    string      `json:"command,omitempty"`
	Subcommand string      `json:"subcommand,omitempty"`
	Actor      string      `json:"actor"`
	AgentType  string      `json:"agent_type,omitempty"`
	PathBucket string      `json:"path_bucket"`
	Input      string      `json:"input"`
	ErrorMsg   string      `json:"error_msg"`
	Suggestion string      `json:"suggestion,omitempty"`
}

// field length limits for FrictionEvent.
const (
	maxInputLength = 500
	maxErrorLength = 200
)

// newFrictionEvent creates a FrictionEvent with current timestamp.
func newFrictionEvent(kind FailureKind) *FrictionEvent {
	return &FrictionEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Kind:      kind,
	}
}

// Truncate enforces field length limits on the event.
// Call this before submission to ensure compliance with API limits.
func (f *FrictionEvent) Truncate() {
	if len(f.Input) > maxInputLength {
		f.Input = f.Input[:maxInputLength]
	}
	if len(f.ErrorMsg) > maxErrorLength {
		f.ErrorMsg = f.ErrorMsg[:maxErrorLength]
	}
}

// MarshalJSON returns JSON bytes ready for transmission.
func (f *FrictionEvent) MarshalJSON() ([]byte, error) {
	type Alias FrictionEvent
	return json.Marshal((*Alias)(f))
}

// Suggestion represents a correction suggestion for a CLI error.
type Suggestion struct {
	Type        SuggestionType
	Original    string
	Corrected   string
	Confidence  float64
	Description string
}

// suggestContext provides context for generating suggestions.
type suggestContext struct {
	Kind         FailureKind
	BadToken     string
	ValidOptions []string
	ParentCmd    string
}

// ParsedError contains structured error information extracted from CLI parsing.
// CLIAdapter implementations parse raw errors into this structure.
type ParsedError struct {
	Kind       FailureKind
	BadToken   string
	Command    string
	Subcommand string
	RawMessage string
}

// CatalogData contains all learned mappings for serialization.
// This is the wire format for catalog updates from the server.
type CatalogData struct {
	Version  string           `json:"version"`
	Commands []CommandMapping `json:"commands"`
	Tokens   []TokenMapping   `json:"tokens"`
}

// CommandMapping represents a full command remap from learned patterns.
type CommandMapping struct {
	Pattern     string  `json:"pattern"`
	Target      string  `json:"target"`
	HasRegex    bool    `json:"has_regex"`
	AutoExecute bool    `json:"auto_execute"`
	Count       int     `json:"count"`
	Confidence  float64 `json:"confidence"`
	Description string  `json:"description"`

	compiledRegex *regexp.Regexp
}

// ApplyMapping applies the mapping to input, returning corrected command.
// For regex patterns, captures are substituted into target ($1, $2, etc).
// Returns the corrected command and true if matched, empty string and false otherwise.
func (m *CommandMapping) ApplyMapping(input string) (string, bool) {
	if !m.HasRegex {
		return m.Target, true
	}

	if m.compiledRegex == nil {
		re, err := regexp.Compile(m.Pattern)
		if err != nil {
			return "", false
		}
		m.compiledRegex = re
	}

	match := m.compiledRegex.FindStringSubmatch(input)
	if match == nil {
		return "", false
	}

	result := m.Target
	for i, capture := range match[1:] {
		placeholder := fmt.Sprintf("$%d", i+1)
		result = strings.ReplaceAll(result, placeholder, capture)
	}

	return result, true
}

// TokenMapping represents a single-token correction from learned patterns.
type TokenMapping struct {
	Pattern    string      `json:"pattern"`
	Target     string      `json:"target"`
	Kind       FailureKind `json:"kind"`
	Count      int         `json:"count"`
	Confidence float64     `json:"confidence"`
}

// SubmitRequest is the request body for friction event submission.
type SubmitRequest struct {
	Version string          `json:"v"`
	Events  []FrictionEvent `json:"events"`
}

// FrictionResponse represents the API response from friction event submission.
type FrictionResponse struct {
	Accepted int          `json:"accepted"`
	Catalog  *CatalogData `json:"catalog,omitempty"`
}
