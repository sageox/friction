package frictionax

import (
	"encoding/json"
	"testing"
)

func TestResult_Format(t *testing.T) {
	tests := []struct {
		name     string
		result   *Result
		jsonMode bool
		wantJSON map[string]any // nil means check wantText instead
		wantText string
	}{
		{
			name:     "nil result returns empty",
			result:   nil,
			jsonMode: false,
			wantText: "",
		},
		{
			name:     "nil result json mode returns empty",
			result:   nil,
			jsonMode: true,
			wantText: "",
		},
		{
			name:     "nil suggestion returns empty",
			result:   &Result{Suggestion: nil},
			jsonMode: false,
			wantText: "",
		},
		{
			name:     "nil suggestion json mode returns empty",
			result:   &Result{Suggestion: nil},
			jsonMode: true,
			wantText: "",
		},
		{
			name: "text mode with suggestion",
			result: &Result{
				Suggestion: &Suggestion{
					Type:       SuggestionLevenshtein,
					Original:   "statu",
					Corrected:  "status",
					Confidence: 0.8,
				},
			},
			jsonMode: false,
			wantText: "Did you mean this?\n    status",
		},
		{
			name: "text mode ignores description",
			result: &Result{
				Suggestion: &Suggestion{
					Type:        SuggestionCommandRemap,
					Original:    "old cmd",
					Corrected:   "new cmd",
					Confidence:  0.95,
					Description: "helpful description",
				},
			},
			jsonMode: false,
			wantText: "Did you mean this?\n    new cmd",
		},
		{
			name: "json mode without description",
			result: &Result{
				Suggestion: &Suggestion{
					Type:       SuggestionLevenshtein,
					Original:   "statu",
					Corrected:  "status",
					Confidence: 0.8,
				},
			},
			jsonMode: true,
			wantJSON: map[string]any{
				"type":       "levenshtein",
				"suggestion": "status",
				"confidence": 0.8,
			},
		},
		{
			name: "json mode with description",
			result: &Result{
				Suggestion: &Suggestion{
					Type:        SuggestionCommandRemap,
					Original:    "daemons list --every",
					Corrected:   "daemons show --all",
					Confidence:  0.95,
					Description: "use show --all instead",
				},
			},
			jsonMode: true,
			wantJSON: map[string]any{
				"type":        "command-remap",
				"suggestion":  "daemons show --all",
				"confidence":  0.95,
				"description": "use show --all instead",
			},
		},
		{
			name: "json mode description omitted when empty",
			result: &Result{
				Suggestion: &Suggestion{
					Type:       SuggestionTokenFix,
					Original:   "depliy",
					Corrected:  "deploy",
					Confidence: 0.9,
				},
			},
			jsonMode: true,
			wantJSON: map[string]any{
				"type":       "token-fix",
				"suggestion": "deploy",
				"confidence": 0.9,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.Format(tt.jsonMode)

			if tt.wantJSON != nil {
				var parsed map[string]any
				if err := json.Unmarshal([]byte(got), &parsed); err != nil {
					t.Fatalf("expected valid JSON, got %q: %v", got, err)
				}
				for k, v := range tt.wantJSON {
					if parsed[k] != v {
						t.Errorf("json[%q] = %v, want %v", k, parsed[k], v)
					}
				}
				// verify description key is absent when not expected
				if _, hasDesc := tt.wantJSON["description"]; !hasDesc {
					if _, gotDesc := parsed["description"]; gotDesc {
						t.Error("description should not be present in JSON output")
					}
				}
			} else {
				if got != tt.wantText {
					t.Errorf("Format() = %q, want %q", got, tt.wantText)
				}
			}
		})
	}
}

func TestResult_Format_JSONStructure(t *testing.T) {
	// verify the JSON only contains expected keys
	r := &Result{
		Suggestion: &Suggestion{
			Type:        SuggestionCommandRemap,
			Original:    "old",
			Corrected:   "new",
			Confidence:  0.99,
			Description: "desc",
		},
	}

	got := r.Format(true)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	allowedKeys := map[string]bool{
		"type": true, "suggestion": true, "confidence": true, "description": true,
	}
	for k := range parsed {
		if !allowedKeys[k] {
			t.Errorf("unexpected key %q in JSON output", k)
		}
	}
}
