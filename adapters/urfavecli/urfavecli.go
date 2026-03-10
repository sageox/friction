// Package urfavecli implements a frictionax.CLIAdapter for the urfave/cli/v2 framework.
//
// It provides command/flag enumeration and structured error parsing, enabling
// the frictionax suggestion engine to offer corrections for CLI usage errors.
package urfavecli

import (
	"regexp"
	"strings"

	"github.com/sageox/frictionax"
	"github.com/urfave/cli/v2"
)

// UrfaveAdapter implements frictionax.CLIAdapter for urfave/cli/v2 apps.
type UrfaveAdapter struct {
	app *cli.App
}

// NewUrfaveAdapter creates a new adapter for the given urfave/cli App.
func NewUrfaveAdapter(app *cli.App) *UrfaveAdapter {
	return &UrfaveAdapter{app: app}
}

// CommandNames returns all available command names.
// Returns names as "subcmd" or "parent subcmd" for nested commands.
// Hidden commands are excluded.
func (a *UrfaveAdapter) CommandNames() []string {
	var names []string
	walkCommands(a.app.Commands, "", &names)
	return names
}

// walkCommands recursively collects command names from urfave/cli command tree.
func walkCommands(cmds []*cli.Command, prefix string, names *[]string) {
	for _, cmd := range cmds {
		if cmd.Hidden {
			continue
		}
		name := cmd.Name
		if prefix != "" {
			name = prefix + " " + name
		}
		*names = append(*names, name)
		// include aliases as separate entries
		for _, alias := range cmd.Aliases {
			aliasName := alias
			if prefix != "" {
				aliasName = prefix + " " + alias
			}
			*names = append(*names, aliasName)
		}
		// recurse into subcommands
		walkCommands(cmd.Subcommands, name, names)
	}
}

// FlagNames returns all available flag names for a command.
// If command is empty, returns global flags from the app.
// Returns flags as "--name" and "-shorthand" where applicable.
func (a *UrfaveAdapter) FlagNames(command string) []string {
	var flags []string

	var targetFlags []cli.Flag
	if command == "" {
		targetFlags = a.app.Flags
	} else {
		cmd := a.findCommand(command)
		if cmd == nil {
			return flags
		}
		targetFlags = cmd.Flags
	}

	for _, f := range targetFlags {
		for _, name := range f.Names() {
			if len(name) == 1 {
				flags = append(flags, "-"+name)
			} else {
				flags = append(flags, "--"+name)
			}
		}
	}

	return flags
}

// ParseError extracts structured info from a urfave/cli error.
// Returns nil if the error is not a parseable urfave/cli error.
//
// Recognized patterns:
//   - "flag provided but not defined: -X" / "flag provided but not defined: --X"
//   - "command \"X\" not found" / various unknown command patterns
//   - "Required flag \"X\" not set" / "Required flags \"X\", \"Y\" not set"
//   - "invalid value \"X\" for flag -Y"
func (a *UrfaveAdapter) ParseError(err error) *frictionax.ParsedError {
	if err == nil {
		return nil
	}

	msg := err.Error()

	// "flag provided but not defined: -X" or "flag provided but not defined: --X"
	if strings.Contains(msg, "flag provided but not defined:") {
		parsed := &frictionax.ParsedError{
			Kind:       frictionax.FailureUnknownFlag,
			RawMessage: msg,
		}
		if idx := strings.Index(msg, "flag provided but not defined: "); idx != -1 {
			token := strings.TrimSpace(msg[idx+31:])
			// strip trailing whitespace or extra text
			if space := strings.Index(token, " "); space != -1 {
				token = token[:space]
			}
			parsed.BadToken = token
		}
		return parsed
	}

	// urfave/cli unknown command: various patterns
	// "command \"X\" not found" (urfave/cli v2 style)
	if isUnknownCommand(msg) {
		parsed := &frictionax.ParsedError{
			Kind:       frictionax.FailureUnknownCommand,
			RawMessage: msg,
		}
		if token := extractQuoted(msg); token != "" {
			parsed.BadToken = token
		}
		return parsed
	}

	// "Required flag \"X\" not set" / "Required flags \"X\", \"Y\" not set"
	if strings.Contains(msg, "Required flag") || strings.Contains(msg, "required flag") {
		return &frictionax.ParsedError{
			Kind:       frictionax.FailureMissingRequired,
			BadToken:   extractQuoted(msg),
			RawMessage: msg,
		}
	}

	// "invalid value \"X\" for flag" patterns
	if strings.Contains(msg, "invalid value") {
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

// isUnknownCommand checks if the error message indicates an unknown command.
func isUnknownCommand(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "command") &&
		(strings.Contains(lower, "not found") || strings.Contains(lower, "unknown"))
}

// findCommand locates a command by name (space-separated for nested commands).
func (a *UrfaveAdapter) findCommand(command string) *cli.Command {
	parts := strings.Split(command, " ")
	cmds := a.app.Commands
	var found *cli.Command

	for _, part := range parts {
		found = nil
		for _, cmd := range cmds {
			if cmd.Name == part || containsAlias(cmd.Aliases, part) {
				found = cmd
				cmds = cmd.Subcommands
				break
			}
		}
		if found == nil {
			return nil
		}
	}
	return found
}

// containsAlias checks if a name exists in a list of aliases.
func containsAlias(aliases []string, name string) bool {
	for _, a := range aliases {
		if a == name {
			return true
		}
	}
	return false
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
var _ frictionax.CLIAdapter = (*UrfaveAdapter)(nil)
