package frictionax

import (
	"encoding/json"
	"fmt"
	"os"
)

// Result contains the outcome of friction handling.
// It includes the suggestion, friction event for analytics, and execution decision.
type Result struct {
	// Suggestion contains the correction suggestion (may be nil if no suggestion found).
	Suggestion *Suggestion

	// Event contains the friction event for analytics submission.
	Event *FrictionEvent

	// AutoExecute is true if the corrected command should be auto-executed.
	// Only true for high-confidence catalog matches.
	AutoExecute bool

	// CorrectedArgs contains the args to re-execute with (if AutoExecute is true).
	CorrectedArgs []string
}

// Format formats the result's suggestion for output.
// If jsonMode is true, returns a JSON object with type, suggestion, and confidence.
// Otherwise returns human-friendly text ("Did you mean this?\n    <suggestion>").
// Returns empty string if no suggestion is present.
func (r *Result) Format(jsonMode bool) string {
	if r == nil || r.Suggestion == nil {
		return ""
	}
	return formatSuggestion(r.Suggestion, jsonMode)
}

// Emit outputs the correction/suggestion for the caller to learn from.
// For agents (jsonMode=true): writes structured JSON to stdout.
// For humans (jsonMode=false): writes human-friendly text to stderr.
func (r *Result) Emit(jsonMode bool) {
	if r == nil || r.Suggestion == nil {
		return
	}

	if jsonMode {
		r.emitJSON()
	} else {
		r.emitText()
	}
}

// EmitError outputs an error with a suggestion for agent learning.
// For suggest-only cases where the command is not auto-executed.
func (r *Result) EmitError(msg string, jsonMode bool) {
	if r == nil || r.Suggestion == nil {
		return
	}

	if jsonMode {
		output := &correctionOutput{
			Error: msg,
			Suggestion: &suggestionMeta{
				Try:  r.Suggestion.Corrected,
				Note: fmt.Sprintf("Use '%s' next time", r.Suggestion.Corrected),
			},
		}
		data, err := json.Marshal(output)
		if err != nil {
			return
		}
		fmt.Fprintln(os.Stdout, string(data))
	} else {
		r.emitText()
	}
}

// WrapOutput wraps successful command output with correction metadata.
// This teaches agents what the correct command is for next time.
func (r *Result) WrapOutput(output any) any {
	if r == nil || r.Suggestion == nil {
		return &correctionOutput{Result: output}
	}

	return &correctionOutput{
		Corrected: &correctionMeta{
			Was:  r.Suggestion.Original,
			Now:  r.Suggestion.Corrected,
			Note: fmt.Sprintf("Use '%s' next time", r.Suggestion.Corrected),
		},
		Result: output,
	}
}

func (r *Result) emitJSON() {
	var output correctionOutput

	if r.AutoExecute {
		output.Corrected = &correctionMeta{
			Was:  r.Suggestion.Original,
			Now:  r.Suggestion.Corrected,
			Note: fmt.Sprintf("Use '%s' next time", r.Suggestion.Corrected),
		}
	} else {
		output.Suggestion = &suggestionMeta{
			Try:  r.Suggestion.Corrected,
			Note: fmt.Sprintf("Use '%s' next time", r.Suggestion.Corrected),
		}
	}

	data, err := json.Marshal(output)
	if err != nil {
		return
	}
	fmt.Fprintln(os.Stdout, string(data))
}

func (r *Result) emitText() {
	if r.AutoExecute {
		fmt.Fprintf(os.Stderr, "-> Correcting to: %s\n", r.Suggestion.Corrected)
	} else {
		fmt.Fprintf(os.Stderr, "Did you mean?\n    %s\n", r.Suggestion.Corrected)
	}
}

// Stats holds telemetry statistics for status display.
type Stats struct {
	Enabled        bool   `json:"enabled"`
	BufferCount    int    `json:"buffer_count"`
	BufferSize     int    `json:"buffer_size"`
	SampleRate     float64 `json:"sample_rate"`
	RetryAfter     any    `json:"retry_after"`
	CatalogVersion string `json:"catalog_version,omitempty"`
}
