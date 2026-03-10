package frictionax

import "strings"

// redactInput joins command arguments and redacts using the provided Redactor.
func redactInput(args []string, redactor Redactor) string {
	input := strings.Join(args, " ")
	if redactor == nil {
		return input
	}
	return redactor.Redact(input)
}

// redactError redacts and truncates an error message.
func redactError(errMsg string, maxLen int, redactor Redactor) string {
	if errMsg == "" {
		return ""
	}
	redacted := errMsg
	if redactor != nil {
		redacted = redactor.Redact(errMsg)
	}
	if maxLen > 0 && len(redacted) > maxLen {
		return redacted[:maxLen]
	}
	return redacted
}
