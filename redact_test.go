package frictionax

import (
	"testing"
)

// passthruRedactor implements Redactor for testing (returns input unchanged).
type passthruRedactor struct{}

func (passthruRedactor) Redact(input string) string {
	return input
}

func TestRedactInput(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		redactor Redactor
		expected string
	}{
		{name: "empty args", args: []string{}, redactor: noOpRedactor{}, expected: ""},
		{name: "single arg no redactor", args: []string{"hello"}, redactor: nil, expected: "hello"},
		{name: "multiple args with noop redactor", args: []string{"git", "commit", "-m", "fix bug"}, redactor: noOpRedactor{}, expected: "git commit -m fix bug"},
		{name: "nil redactor returns joined args", args: []string{"deploy", "--env=prod"}, redactor: nil, expected: "deploy --env=prod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := redactInput(tt.args, tt.redactor)
			if result != tt.expected {
				t.Errorf("redactInput() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestRedactError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		maxLen   int
		redactor Redactor
		expected string
	}{
		{name: "empty error message", errMsg: "", maxLen: 100, redactor: noOpRedactor{}, expected: ""},
		{name: "short message no secrets", errMsg: "connection failed", maxLen: 100, redactor: noOpRedactor{}, expected: "connection failed"},
		{name: "truncation without redactor", errMsg: "this is a very long error message that should be truncated", maxLen: 20, redactor: nil, expected: "this is a very long "},
		{name: "zero maxLen no truncation", errMsg: "this message should not be truncated at all", maxLen: 0, redactor: noOpRedactor{}, expected: "this message should not be truncated at all"},
		{name: "negative maxLen no truncation", errMsg: "negative maxLen should not truncate", maxLen: -1, redactor: noOpRedactor{}, expected: "negative maxLen should not truncate"},
		{name: "message shorter than maxLen", errMsg: "short", maxLen: 100, redactor: noOpRedactor{}, expected: "short"},
		{name: "message exactly at maxLen", errMsg: "exactly10!", maxLen: 10, redactor: noOpRedactor{}, expected: "exactly10!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := redactError(tt.errMsg, tt.maxLen, tt.redactor)
			if result != tt.expected {
				t.Errorf("redactError() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestNoOpRedactor(t *testing.T) {
	r := noOpRedactor{}
	input := "some sensitive data: AKIAIOSFODNN7EXAMPLE"
	if got := r.Redact(input); got != input {
		t.Errorf("noOpRedactor.Redact() = %q, want %q", got, input)
	}
}
