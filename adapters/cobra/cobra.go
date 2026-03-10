// Package cobra implements a frictionax.CLIAdapter for the spf13/cobra CLI framework.
//
// It provides command/flag enumeration and structured error parsing, enabling
// the frictionax suggestion engine to offer corrections for CLI usage errors.
package cobra

import (
	"regexp"
	"strings"

	"github.com/sageox/frictionax"
	spfcobra "github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// CobraAdapter implements frictionax.CLIAdapter for spf13/cobra command trees.
type CobraAdapter struct {
	root *spfcobra.Command
}

// NewCobraAdapter creates a new adapter for the given root command.
func NewCobraAdapter(root *spfcobra.Command) *CobraAdapter {
	return &CobraAdapter{root: root}
}

// CommandNames returns all available command names.
// Returns names as "subcmd" or "parent subcmd" for nested commands.
// Hidden and deprecated commands are excluded.
func (a *CobraAdapter) CommandNames() []string {
	var names []string
	walkCommands(a.root, "", func(cmd *spfcobra.Command, prefix string) {
		if cmd == a.root {
			return
		}
		if cmd.Hidden || cmd.Deprecated != "" {
			return
		}
		name := cmd.Name()
		if prefix != "" {
			name = prefix + " " + name
		}
		names = append(names, name)
	})
	return names
}

// FlagNames returns all available flag names for a command.
// If command is empty, returns global (persistent) flags from root.
// Returns flags as "--name" and "-shorthand" if shorthand is available.
func (a *CobraAdapter) FlagNames(command string) []string {
	var flags []string

	cmd := a.findCommand(command)
	if cmd == nil {
		return flags
	}

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		flags = append(flags, "--"+f.Name)
		if f.Shorthand != "" {
			flags = append(flags, "-"+f.Shorthand)
		}
	})

	return flags
}

// ParseError extracts structured info from a Cobra CLI error.
// Returns nil if the error is not a parseable Cobra error.
//
// Recognized patterns:
//   - "unknown command \"X\" for \"parent\""
//   - "unknown flag: --X"
//   - "unknown shorthand flag: 'X' in -X"
//   - "required flag(s) \"X\", \"Y\" not set"
//   - "invalid argument \"X\" for \"--flag\""
func (a *CobraAdapter) ParseError(err error) *frictionax.ParsedError {
	if err == nil {
		return nil
	}

	msg := err.Error()

	// "unknown command \"X\" for \"parent\""
	if strings.Contains(msg, "unknown command") {
		parsed := &frictionax.ParsedError{
			Kind:       frictionax.FailureUnknownCommand,
			RawMessage: msg,
		}
		if token := extractQuoted(msg); token != "" {
			parsed.BadToken = token
		}
		// extract parent command from "for \"parent\""
		if idx := strings.Index(msg, "\" for \""); idx != -1 {
			rest := msg[idx+7:]
			if end := strings.Index(rest, "\""); end != -1 {
				parsed.Command = rest[:end]
			}
		}
		return parsed
	}

	// "unknown flag: --X"
	if strings.Contains(msg, "unknown flag:") {
		parsed := &frictionax.ParsedError{
			Kind:       frictionax.FailureUnknownFlag,
			RawMessage: msg,
		}
		if idx := strings.Index(msg, "unknown flag: "); idx != -1 {
			token := strings.TrimSpace(msg[idx+14:])
			if space := strings.Index(token, " "); space != -1 {
				token = token[:space]
			}
			parsed.BadToken = token
		}
		return parsed
	}

	// "unknown shorthand flag: 'X' in -X"
	if strings.Contains(msg, "unknown shorthand flag:") {
		parsed := &frictionax.ParsedError{
			Kind:       frictionax.FailureUnknownFlag,
			RawMessage: msg,
		}
		if token := extractSingleQuoted(msg); token != "" {
			parsed.BadToken = "-" + token
		}
		return parsed
	}

	// "required flag(s) \"X\", \"Y\" not set"
	if strings.Contains(msg, "required flag") {
		return &frictionax.ParsedError{
			Kind:       frictionax.FailureMissingRequired,
			BadToken:   extractQuoted(msg),
			RawMessage: msg,
		}
	}

	// "invalid argument \"X\" for \"--flag\""
	if strings.Contains(msg, "invalid argument") {
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

// walkCommands recursively walks the command tree calling fn for each command.
func walkCommands(cmd *spfcobra.Command, prefix string, fn func(*spfcobra.Command, string)) {
	fn(cmd, prefix)
	for _, child := range cmd.Commands() {
		childPrefix := prefix
		if cmd.Name() != "" && prefix == "" {
			childPrefix = cmd.Name()
		} else if prefix != "" {
			childPrefix = prefix + " " + cmd.Name()
		}
		// for root command, prefix stays empty for direct children
		if cmd.Parent() == nil {
			childPrefix = ""
		}
		walkCommands(child, childPrefix, fn)
	}
}

// findCommand locates a command by name (space-separated for nested commands).
// If command is empty, returns the root command.
func (a *CobraAdapter) findCommand(command string) *spfcobra.Command {
	if command == "" {
		return a.root
	}

	parts := strings.Split(command, " ")
	cmd := a.root
	for _, part := range parts {
		found := false
		for _, child := range cmd.Commands() {
			if child.Name() == part {
				cmd = child
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return cmd
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

// extractSingleQuoted extracts the first single-quoted string from a message.
func extractSingleQuoted(msg string) string {
	re := regexp.MustCompile(`'([^']+)'`)
	match := re.FindStringSubmatch(msg)
	if len(match) >= 2 {
		return match[1]
	}
	return ""
}

// compile-time interface check
var _ frictionax.CLIAdapter = (*CobraAdapter)(nil)
