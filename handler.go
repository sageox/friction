package frictionax

import (
	"encoding/json"
	"os"
	"strings"
)

// autoExecuteThreshold is the minimum confidence for auto-execute.
const autoExecuteThreshold = 0.85

// pathBucket values for categorizing working directory.
const (
	pathBucketHome  = "home"
	pathBucketRepo  = "repo"
	pathBucketOther = "other"
)

// ActorDetector detects whether the current context is a human, AI agent, or CI.
type ActorDetector interface {
	DetectActor() (Actor, string)
}

// Redactor redacts sensitive information from strings.
type Redactor interface {
	Redact(input string) string
}

// noOpRedactor returns input unchanged.
type noOpRedactor struct{}

func (noOpRedactor) Redact(input string) string {
	return input
}

// envActorDetector is the default actor detector that checks environment variables.
type envActorDetector struct{}

func (envActorDetector) DetectActor() (Actor, string) {
	if ci := os.Getenv("CI"); ci != "" {
		return ActorAgent, "ci"
	}
	return ActorHuman, ""
}

// handler processes CLI errors and generates friction events with suggestions.
// This is the internal implementation; the public API is Friction.Handle().
type handler struct {
	adapter       CLIAdapter
	engine        *suggestionEngine
	actorDetector ActorDetector
	redactor      Redactor
}

// newHandler creates a handler with the given adapter, catalog, actor detector, and redactor.
func newHandler(adapter CLIAdapter, cat catalog, actorDetector ActorDetector, redactor Redactor) *handler {
	if actorDetector == nil {
		actorDetector = envActorDetector{}
	}
	if redactor == nil {
		redactor = noOpRedactor{}
	}
	return &handler{
		adapter:       adapter,
		engine:        newSuggestionEngine(cat),
		actorDetector: actorDetector,
		redactor:      redactor,
	}
}

// HandleWithAutoExecute processes CLI args and error, returning a Result.
func (h *handler) HandleWithAutoExecute(args []string, err error) *Result {
	parsed := h.adapter.ParseError(err)
	if parsed == nil {
		return nil
	}

	fullCommand := strings.Join(args, " ")

	var validOptions []string
	switch parsed.Kind {
	case FailureUnknownCommand:
		validOptions = h.adapter.CommandNames()
	case FailureUnknownFlag:
		validOptions = h.adapter.FlagNames(parsed.Command)
	default:
		validOptions = h.adapter.CommandNames()
	}

	ctx := suggestContext{
		Kind:         parsed.Kind,
		BadToken:     parsed.BadToken,
		ValidOptions: validOptions,
		ParentCmd:    parsed.Command,
	}
	suggestion, mapping := h.engine.suggestForCommandWithMapping(fullCommand, ctx)

	actor, agentType := h.actorDetector.DetectActor()

	event := newFrictionEvent(parsed.Kind)
	event.Command = parsed.Command
	event.Subcommand = parsed.Subcommand
	event.Actor = string(actor)
	if actor == ActorAgent && agentType != "" {
		event.AgentType = agentType
	}
	event.PathBucket = detectPathBucket()
	event.Input = redactInput(args, h.redactor)
	event.ErrorMsg = redactError(parsed.RawMessage, maxErrorLength, h.redactor)
	event.Truncate()

	autoExecute := false
	var correctedArgs []string

	if suggestion != nil && mapping != nil {
		if mapping.AutoExecute && suggestion.Confidence >= autoExecuteThreshold {
			autoExecute = true
			correctedArgs = parseArgs(suggestion.Corrected)
		}
	}

	return &Result{
		Suggestion:    suggestion,
		Event:         event,
		AutoExecute:   autoExecute,
		CorrectedArgs: correctedArgs,
	}
}

// parseArgs splits a command string into args.
func parseArgs(command string) []string {
	return strings.Fields(command)
}

// detectPathBucket categorizes the current working directory.
func detectPathBucket() string {
	cwd, err := os.Getwd()
	if err != nil {
		return pathBucketOther
	}

	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(cwd, home) {
		if isGitRepo(cwd) {
			return pathBucketRepo
		}
		return pathBucketHome
	}

	if isGitRepo(cwd) {
		return pathBucketRepo
	}

	return pathBucketOther
}

func isGitRepo(dir string) bool {
	for {
		gitPath := dir + "/.git"
		if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
			return true
		}

		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == "" || parent == dir {
			break
		}
		dir = parent
	}
	return false
}

// formatSuggestion formats a suggestion for output.
func formatSuggestion(s *Suggestion, jsonMode bool) string {
	if s == nil {
		return ""
	}

	if jsonMode {
		output := map[string]any{
			"type":       string(s.Type),
			"suggestion": s.Corrected,
			"confidence": s.Confidence,
		}
		if s.Description != "" {
			output["description"] = s.Description
		}

		data, err := json.Marshal(output)
		if err != nil {
			return ""
		}
		return string(data)
	}

	return "Did you mean this?\n    " + s.Corrected
}
