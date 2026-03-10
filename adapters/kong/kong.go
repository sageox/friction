// Package kong implements a frictionax.CLIAdapter for the alecthomas/kong CLI framework.
//
// It provides command/flag enumeration and structured error parsing, enabling
// the frictionax suggestion engine to offer corrections for CLI usage errors.
package kong

import (
	"regexp"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/sageox/frictionax"
)

// KongAdapter implements frictionax.CLIAdapter for alecthomas/kong apps.
type KongAdapter struct {
	app *kong.Kong
}

// NewKongAdapter creates a new adapter for the given kong.Kong instance.
func NewKongAdapter(app *kong.Kong) *KongAdapter {
	return &KongAdapter{app: app}
}

// CommandNames returns all available command names.
// Returns names as "subcmd" or "parent subcmd" for nested commands.
// Hidden commands are excluded.
func (a *KongAdapter) CommandNames() []string {
	var names []string
	if a.app.Model == nil {
		return names
	}
	walkNode(a.app.Model.Node, "", &names)
	return names
}

// walkNode recursively collects command names from kong's node tree.
func walkNode(node *kong.Node, prefix string, names *[]string) {
	for _, child := range node.Children {
		if child == nil || child.Hidden {
			continue
		}
		// only include command nodes (not argument or flag nodes)
		if child.Type != kong.CommandNode {
			continue
		}
		name := child.Name
		if prefix != "" {
			name = prefix + " " + name
		}
		*names = append(*names, name)
		walkNode(child, name, names)
	}
}

// FlagNames returns all available flag names for a command.
// If command is empty, returns global flags from the root node.
// Returns flags as "--name" and "-shorthand" where applicable.
func (a *KongAdapter) FlagNames(command string) []string {
	var flags []string
	if a.app.Model == nil {
		return flags
	}

	node := a.findNode(command)
	if node == nil {
		return flags
	}

	for _, flag := range node.Flags {
		if flag.Hidden {
			continue
		}
		flags = append(flags, "--"+flag.Name)
		if flag.Short != 0 {
			flags = append(flags, "-"+string(flag.Short))
		}
	}

	return flags
}

// ParseError extracts structured info from a Kong CLI error.
// Returns nil if the error is not a parseable Kong error.
//
// Recognized patterns:
//   - "unknown command \"X\"" / "unexpected argument X"
//   - "unknown flag --X"
//   - "missing flags: --X, --Y" / "<arg> is required"
//   - "X: invalid value" / various type conversion errors
func (a *KongAdapter) ParseError(err error) *frictionax.ParsedError {
	if err == nil {
		return nil
	}

	msg := err.Error()

	// "unknown flag --X" or "unknown flag -X"
	if strings.Contains(msg, "unknown flag") {
		parsed := &frictionax.ParsedError{
			Kind:       frictionax.FailureUnknownFlag,
			RawMessage: msg,
		}
		if idx := strings.Index(msg, "unknown flag "); idx != -1 {
			token := strings.TrimSpace(msg[idx+13:])
			// strip trailing whitespace or extra text
			if space := strings.Index(token, " "); space != -1 {
				token = token[:space]
			}
			parsed.BadToken = token
		}
		return parsed
	}

	// unknown command patterns:
	// "unknown command \"X\"" or "unexpected argument X"
	if strings.Contains(msg, "unknown command") || strings.Contains(msg, "unexpected argument") {
		parsed := &frictionax.ParsedError{
			Kind:       frictionax.FailureUnknownCommand,
			RawMessage: msg,
		}
		// try quoted first
		if token := extractQuoted(msg); token != "" {
			parsed.BadToken = token
		} else if strings.Contains(msg, "unexpected argument") {
			// "unexpected argument X" - extract trailing word
			if idx := strings.Index(msg, "unexpected argument "); idx != -1 {
				token := strings.TrimSpace(msg[idx+20:])
				if space := strings.Index(token, " "); space != -1 {
					token = token[:space]
				}
				parsed.BadToken = token
			}
		}
		return parsed
	}

	// missing required: "missing flags: --X" or "<arg> is required"
	if strings.Contains(msg, "missing flags") || strings.Contains(msg, "is required") {
		parsed := &frictionax.ParsedError{
			Kind:       frictionax.FailureMissingRequired,
			RawMessage: msg,
		}
		if token := extractQuoted(msg); token != "" {
			parsed.BadToken = token
		} else if strings.Contains(msg, "missing flags: ") {
			// extract the flag names from "missing flags: --X, --Y"
			if idx := strings.Index(msg, "missing flags: "); idx != -1 {
				token := strings.TrimSpace(msg[idx+15:])
				// take first flag
				if comma := strings.Index(token, ","); comma != -1 {
					token = token[:comma]
				}
				parsed.BadToken = strings.TrimSpace(token)
			}
		}
		return parsed
	}

	// invalid value / type errors
	if strings.Contains(msg, "invalid value") || isTypeConversionError(msg) {
		parsed := &frictionax.ParsedError{
			Kind:       frictionax.FailureInvalidArg,
			RawMessage: msg,
		}
		if token := extractQuoted(msg); token != "" {
			parsed.BadToken = token
		}
		return parsed
	}

	return nil
}

// isTypeConversionError checks for common Go type conversion error patterns
// that kong surfaces from strconv or similar.
func isTypeConversionError(msg string) bool {
	return strings.Contains(msg, "strconv.") ||
		strings.Contains(msg, "expected") && strings.Contains(msg, "got") ||
		strings.Contains(msg, "can't be") ||
		strings.Contains(msg, "invalid")
}

// findNode locates a command node by name (space-separated for nested commands).
// If command is empty, returns the root node.
func (a *KongAdapter) findNode(command string) *kong.Node {
	if a.app.Model == nil {
		return nil
	}
	if command == "" {
		return a.app.Model.Node
	}

	parts := strings.Split(command, " ")
	node := a.app.Model.Node
	for _, part := range parts {
		found := false
		for _, child := range node.Children {
			if child != nil && child.Name == part {
				node = child
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return node
}

// extractQuoted extracts the first double-quoted string from a message.
func extractQuoted(msg string) string {
	re := regexp.MustCompile(`"([^"]+)"`)
	match := re.FindStringSubmatch(msg)
	if len(match) >= 2 {
		return match[1]
	}
	return ""
}

// compile-time interface check
var _ frictionax.CLIAdapter = (*KongAdapter)(nil)
