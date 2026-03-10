package frictionax

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stderr = w

	f()

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestEmitCorrection_JSONMode_AutoExecute(t *testing.T) {
	result := &Result{
		Suggestion: &Suggestion{
			Type:       SuggestionCommandRemap,
			Original:   "mycli agent prine",
			Corrected:  "mycli agent prime",
			Confidence: 0.95,
		},
		AutoExecute: true,
	}

	output := captureStdout(t, func() {
		result.Emit(true)
	})

	var parsed correctionOutput
	err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed)
	if err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}

	if parsed.Corrected == nil {
		t.Fatal("should have _corrected field")
	}
	if parsed.Corrected.Was != "mycli agent prine" {
		t.Errorf("was = %q, want %q", parsed.Corrected.Was, "mycli agent prine")
	}
	if parsed.Corrected.Now != "mycli agent prime" {
		t.Errorf("now = %q, want %q", parsed.Corrected.Now, "mycli agent prime")
	}
	if !strings.Contains(parsed.Corrected.Note, "mycli agent prime") {
		t.Errorf("note should contain corrected command")
	}
	if parsed.Suggestion != nil {
		t.Error("should not have _suggestion for auto-execute")
	}
}

func TestEmitCorrection_JSONMode_SuggestOnly(t *testing.T) {
	result := &Result{
		Suggestion: &Suggestion{
			Type:       SuggestionLevenshtein,
			Original:   "mycli agent statsu",
			Corrected:  "mycli agent status",
			Confidence: 0.75,
		},
		AutoExecute: false,
	}

	output := captureStdout(t, func() {
		result.Emit(true)
	})

	var parsed correctionOutput
	err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed)
	if err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}

	if parsed.Suggestion == nil {
		t.Fatal("should have _suggestion field")
	}
	if parsed.Suggestion.Try != "mycli agent status" {
		t.Errorf("try = %q, want %q", parsed.Suggestion.Try, "mycli agent status")
	}
	if parsed.Corrected != nil {
		t.Error("should not have _corrected for suggest-only")
	}
}

func TestEmitCorrection_TextMode_AutoExecute(t *testing.T) {
	result := &Result{
		Suggestion: &Suggestion{
			Type:       SuggestionCommandRemap,
			Original:   "mycli agent prine",
			Corrected:  "mycli agent prime",
			Confidence: 0.95,
		},
		AutoExecute: true,
	}

	output := captureStderr(t, func() {
		result.Emit(false)
	})

	if !strings.Contains(output, "Correcting to:") {
		t.Errorf("output should contain 'Correcting to:', got %q", output)
	}
	if !strings.Contains(output, "mycli agent prime") {
		t.Errorf("output should contain corrected command, got %q", output)
	}
}

func TestEmitCorrection_TextMode_SuggestOnly(t *testing.T) {
	result := &Result{
		Suggestion: &Suggestion{
			Type:       SuggestionLevenshtein,
			Original:   "mycli agent statsu",
			Corrected:  "mycli agent status",
			Confidence: 0.75,
		},
		AutoExecute: false,
	}

	output := captureStderr(t, func() {
		result.Emit(false)
	})

	if !strings.Contains(output, "Did you mean?") {
		t.Errorf("output should contain 'Did you mean?', got %q", output)
	}
	if !strings.Contains(output, "mycli agent status") {
		t.Errorf("output should contain suggestion, got %q", output)
	}
}

func TestEmitCorrection_NilResult(t *testing.T) {
	var nilResult *Result
	stdout := captureStdout(t, func() {
		nilResult.Emit(true)
	})
	stderr := captureStderr(t, func() {
		nilResult.Emit(false)
	})

	if stdout != "" {
		t.Errorf("nil result should produce no stdout, got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("nil result should produce no stderr, got %q", stderr)
	}
}

func TestEmitCorrection_NilSuggestion(t *testing.T) {
	result := &Result{
		Suggestion:  nil,
		AutoExecute: false,
	}

	stdout := captureStdout(t, func() {
		result.Emit(true)
	})
	stderr := captureStderr(t, func() {
		result.Emit(false)
	})

	if stdout != "" {
		t.Errorf("nil suggestion should produce no stdout, got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("nil suggestion should produce no stderr, got %q", stderr)
	}
}

func TestEmitSuggestionError_JSONMode(t *testing.T) {
	result := &Result{
		Suggestion: &Suggestion{
			Type:       SuggestionLevenshtein,
			Original:   "mycli agent statsu",
			Corrected:  "mycli agent status",
			Confidence: 0.75,
		},
		AutoExecute: false,
	}

	output := captureStdout(t, func() {
		result.EmitError("unknown command: statsu", true)
	})

	var parsed correctionOutput
	err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed)
	if err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}

	if parsed.Error != "unknown command: statsu" {
		t.Errorf("error = %q, want %q", parsed.Error, "unknown command: statsu")
	}
	if parsed.Suggestion == nil {
		t.Fatal("should have _suggestion field")
	}
	if parsed.Suggestion.Try != "mycli agent status" {
		t.Errorf("try = %q, want %q", parsed.Suggestion.Try, "mycli agent status")
	}
}

func TestEmitSuggestionError_NilResult(t *testing.T) {
	var nilResult *Result
	stdout := captureStdout(t, func() {
		nilResult.EmitError("some error", true)
	})
	stderr := captureStderr(t, func() {
		nilResult.EmitError("some error", false)
	})

	if stdout != "" {
		t.Errorf("nil result should produce no stdout, got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("nil result should produce no stderr, got %q", stderr)
	}
}

func TestWrapOutputWithCorrection(t *testing.T) {
	tests := []struct {
		name          string
		result        *Result
		commandOutput any
		wantCorrected bool
		wantWas       string
		wantNow       string
	}{
		{
			name: "wraps output with correction metadata",
			result: &Result{
				Suggestion: &Suggestion{
					Original:  "mycli agent prine",
					Corrected: "mycli agent prime",
				},
				AutoExecute: true,
			},
			commandOutput: map[string]string{"agent_id": "Oxa123"},
			wantCorrected: true,
			wantWas:       "mycli agent prine",
			wantNow:       "mycli agent prime",
		},
		{
			name:          "nil result returns just output",
			result:        nil,
			commandOutput: map[string]string{"status": "ok"},
			wantCorrected: false,
		},
		{
			name: "nil suggestion returns just output",
			result: &Result{
				Suggestion:  nil,
				AutoExecute: false,
			},
			commandOutput: "plain string output",
			wantCorrected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := tt.result.WrapOutput(tt.commandOutput)
			if output == nil {
				t.Fatal("output should not be nil")
			}

			co, ok := output.(*correctionOutput)
			if !ok {
				t.Fatalf("output should be *correctionOutput, got %T", output)
			}

			if tt.wantCorrected {
				if co.Corrected == nil {
					t.Fatal("should have Corrected field")
				}
				if co.Corrected.Was != tt.wantWas {
					t.Errorf("was = %q, want %q", co.Corrected.Was, tt.wantWas)
				}
				if co.Corrected.Now != tt.wantNow {
					t.Errorf("now = %q, want %q", co.Corrected.Now, tt.wantNow)
				}
				if !strings.Contains(co.Corrected.Note, tt.wantNow) {
					t.Errorf("note should contain %q", tt.wantNow)
				}
			} else {
				if co.Corrected != nil {
					t.Error("should not have Corrected field")
				}
			}
		})
	}
}

func TestCorrectionOutput_OmitsEmptyFields(t *testing.T) {
	output := &correctionOutput{
		Result: "test",
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	s := string(data)
	if strings.Contains(s, "_corrected") {
		t.Error("should not include _corrected")
	}
	if strings.Contains(s, "_suggestion") {
		t.Error("should not include _suggestion")
	}
	if strings.Contains(s, "error") {
		t.Error("should not include error")
	}
}
