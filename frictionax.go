package frictionax

import (
	"log/slog"
	"net/http"
	"strings"
)

// Friction is the single entry point for CLI friction detection, correction,
// and telemetry. Create one with New() and configure via options.
//
// Basic usage (suggestions only, no telemetry):
//
//	f := frictionax.New(adapter)
//	result := f.Handle(args, err)
//
// Full usage (suggestions + telemetry):
//
//	f := frictionax.New(adapter,
//	    frictionax.WithCatalog("mycli"),
//	    frictionax.WithTelemetry("https://api.example.com", "0.5.0"),
//	    frictionax.WithAuth(func() string { return token }),
//	)
//	defer f.Close()
//	result := f.Handle(args, err)
type Friction struct {
	adapter       CLIAdapter
	engine        *suggestionEngine
	collector     *frictionCollector // nil if no telemetry
	actorDetector ActorDetector
	redactor      Redactor
	logger        *slog.Logger
	catalog       *frictionCatalog
}

// frictionConfig holds configuration gathered from options before constructing Friction.
type frictionConfig struct {
	cliName          string
	endpoint         string
	version          string
	authFunc         func() string
	redactor         Redactor
	actorDetector    ActorDetector
	logger           *slog.Logger
	cachePath        string
	requestDecorator func(*http.Request)
	isEnabled        func() bool
}

// Option configures the Friction instance.
type Option func(*frictionConfig)

// WithCatalog enables the learned-corrections catalog for the given CLI name.
// The catalog provides high-confidence corrections from server-side data.
func WithCatalog(cliName string) Option {
	return func(c *frictionConfig) {
		c.cliName = cliName
	}
}

// WithTelemetry enables background telemetry reporting.
// Events are buffered and sent to the endpoint periodically.
func WithTelemetry(endpoint, version string) Option {
	return func(c *frictionConfig) {
		c.endpoint = endpoint
		c.version = version
	}
}

// WithAuth sets the authentication function for telemetry requests.
// The function is called on each request and should return a bearer token.
func WithAuth(fn func() string) Option {
	return func(c *frictionConfig) {
		c.authFunc = fn
	}
}

// WithRedactor sets a custom redactor for input and error message sanitization.
// If not set, inputs are passed through unmodified.
func WithRedactor(r Redactor) Option {
	return func(c *frictionConfig) {
		c.redactor = r
	}
}

// WithActorDetector sets a custom actor detector.
// If not set, the default environment-based detector is used.
func WithActorDetector(d ActorDetector) Option {
	return func(c *frictionConfig) {
		c.actorDetector = d
	}
}

// WithLogger sets a structured logger for debug output.
// If not set, slog.Default() is used.
func WithLogger(l *slog.Logger) Option {
	return func(c *frictionConfig) {
		c.logger = l
	}
}

// WithCachePath sets the file path for on-disk catalog caching.
// If not set, no catalog caching is performed.
func WithCachePath(path string) Option {
	return func(c *frictionConfig) {
		c.cachePath = path
	}
}

// WithRequestDecorator sets a function called on each outgoing HTTP request.
// Use this to add custom headers (e.g., User-Agent).
func WithRequestDecorator(fn func(*http.Request)) Option {
	return func(c *frictionConfig) {
		c.requestDecorator = fn
	}
}

// WithIsEnabled sets a function that controls whether telemetry is active.
// If the function returns false, events are silently dropped.
func WithIsEnabled(fn func() bool) Option {
	return func(c *frictionConfig) {
		c.isEnabled = fn
	}
}

// New creates a Friction instance with the given adapter and options.
// The adapter is required and provides CLI framework integration.
//
// Call Close() when done to flush any buffered telemetry events.
func New(adapter CLIAdapter, opts ...Option) *Friction {
	cfg := &frictionConfig{
		redactor:      noOpRedactor{},
		actorDetector: envActorDetector{},
		logger:        slog.Default(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	// build catalog if configured
	var cat catalog
	var fc *frictionCatalog
	if cfg.cliName != "" {
		fc = newFrictionCatalog(cfg.cliName)
		cat = fc
	}

	f := &Friction{
		adapter:       adapter,
		engine:        newSuggestionEngine(cat),
		actorDetector: cfg.actorDetector,
		redactor:      cfg.redactor,
		logger:        cfg.logger,
		catalog:       fc,
	}

	// build collector if telemetry is configured
	if cfg.endpoint != "" {
		collCfg := collectorConfig{
			Endpoint:         cfg.endpoint,
			Version:          cfg.version,
			AuthFunc:         cfg.authFunc,
			RequestDecorator: cfg.requestDecorator,
			Logger:           cfg.logger,
			CachePath:        cfg.cachePath,
			IsEnabled:        cfg.isEnabled,
		}
		f.collector = newFrictionCollector(collCfg)
		f.collector.Start()

		// if collector has cached catalog data, load it into our catalog
		if fc != nil && f.collector.catalogCache != nil {
			if data := f.collector.CatalogData(); data != nil {
				_ = fc.Update(*data)
			}
		}
	}

	return f
}

// Handle processes CLI args and error, returning a Result with suggestion
// and execution decision. Returns nil if the error cannot be parsed.
func (f *Friction) Handle(args []string, err error) *Result {
	parsed := f.adapter.ParseError(err)
	if parsed == nil {
		return nil
	}

	fullCommand := strings.Join(args, " ")

	// get valid options based on failure kind
	var validOptions []string
	switch parsed.Kind {
	case FailureUnknownCommand:
		validOptions = f.adapter.CommandNames()
	case FailureUnknownFlag:
		validOptions = f.adapter.FlagNames(parsed.Command)
	default:
		validOptions = f.adapter.CommandNames()
	}

	// get suggestion from engine
	ctx := suggestContext{
		Kind:         parsed.Kind,
		BadToken:     parsed.BadToken,
		ValidOptions: validOptions,
		ParentCmd:    parsed.Command,
	}
	suggestion, mapping := f.engine.suggestForCommandWithMapping(fullCommand, ctx)

	// detect actor
	actor, agentType := f.actorDetector.DetectActor()

	// build friction event
	event := newFrictionEvent(parsed.Kind)
	event.Command = parsed.Command
	event.Subcommand = parsed.Subcommand
	event.Actor = string(actor)
	if actor == ActorAgent && agentType != "" {
		event.AgentType = agentType
	}
	event.PathBucket = detectPathBucket()
	event.Input = redactInput(args, f.redactor)
	event.ErrorMsg = redactError(parsed.RawMessage, maxErrorLength, f.redactor)
	event.Truncate()

	// determine action
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

// Record adds a friction event directly to the telemetry buffer.
// This is useful for recording events that aren't CLI parse errors.
// No-op if telemetry is not configured.
func (f *Friction) Record(event FrictionEvent) {
	if f.collector == nil {
		return
	}
	f.collector.Record(event)
}

// Close stops background telemetry processing and flushes buffered events.
// Safe to call multiple times. No-op if telemetry is not configured.
func (f *Friction) Close() {
	if f.collector == nil {
		return
	}
	f.collector.Stop()
}

// UpdateCatalog replaces the catalog data with the provided data.
// No-op if the catalog is not configured.
func (f *Friction) UpdateCatalog(data CatalogData) error {
	if f.catalog == nil {
		return nil
	}
	return f.catalog.Update(data)
}

// Stats returns current telemetry statistics.
// Returns zero-value Stats if telemetry is not configured.
func (f *Friction) Stats() Stats {
	if f.collector == nil {
		return Stats{}
	}
	s := f.collector.stats()
	return Stats{
		Enabled:        s.Enabled,
		BufferCount:    s.BufferCount,
		BufferSize:     s.BufferSize,
		SampleRate:     s.SampleRate,
		RetryAfter:     s.RetryAfter,
		CatalogVersion: s.CatalogVersion,
	}
}
