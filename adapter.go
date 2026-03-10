package frictionax

// CLIAdapter abstracts CLI framework details for error parsing.
// Implement this for your CLI framework (Cobra, urfave/cli, Kong, etc.).
type CLIAdapter interface {
	// CommandNames returns all available command names.
	CommandNames() []string

	// FlagNames returns all available flag names for a command.
	// If command is empty, returns global flags.
	FlagNames(command string) []string

	// ParseError extracts structured info from a CLI error.
	// Returns nil if the error is not a parseable CLI error.
	ParseError(err error) *ParsedError
}
