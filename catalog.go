package frictionax

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// catalog provides lookup for learned command and token mappings.
type catalog interface {
	LookupCommand(input string) *CommandMapping
	LookupToken(token string, kind FailureKind) *TokenMapping
	Update(data CatalogData) error
	Version() string
}

// frictionCatalog implements catalog with thread-safe in-memory storage.
type frictionCatalog struct {
	mu            sync.RWMutex
	cliName       string
	version       string
	commands      map[string]*CommandMapping
	regexCommands []*CommandMapping
	tokens        map[string]*TokenMapping
}

// newFrictionCatalog creates an empty frictionCatalog ready for updates.
func newFrictionCatalog(cliName string) *frictionCatalog {
	return &frictionCatalog{
		cliName:       cliName,
		version:       "",
		commands:      make(map[string]*CommandMapping),
		regexCommands: nil,
		tokens:        make(map[string]*TokenMapping),
	}
}

// LookupCommand finds a command mapping for the given input.
func (c *frictionCatalog) LookupCommand(input string) *CommandMapping {
	c.mu.RLock()
	defer c.mu.RUnlock()

	normalized := c.normalizeCommand(input)
	if mapping := c.commands[normalized]; mapping != nil {
		return mapping
	}

	normalizedInput := normalized
	if normalizedInput == "" {
		normalizedInput = strings.TrimPrefix(strings.TrimSpace(input), c.cliName+" ")
	}

	for _, mapping := range c.regexCommands {
		if mapping.compiledRegex == nil {
			continue
		}
		if mapping.compiledRegex.MatchString(normalizedInput) {
			return mapping
		}
	}

	return nil
}

// LookupToken finds a token mapping for the given token and failure kind.
func (c *frictionCatalog) LookupToken(token string, kind FailureKind) *TokenMapping {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := tokenKey(token, kind)
	return c.tokens[key]
}

// Update replaces all catalog data with new data.
func (c *frictionCatalog) Update(data CatalogData) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	commands := make(map[string]*CommandMapping)
	var regexCommands []*CommandMapping

	for i := range data.Commands {
		mapping := &data.Commands[i]
		if mapping.HasRegex {
			re, err := regexp.Compile(mapping.Pattern)
			if err != nil {
				continue
			}
			mapping.compiledRegex = re
			regexCommands = append(regexCommands, mapping)
		} else {
			normalized := c.normalizeCommand(mapping.Pattern)
			commands[normalized] = mapping
		}
	}

	tokens := make(map[string]*TokenMapping, len(data.Tokens))
	for i := range data.Tokens {
		mapping := &data.Tokens[i]
		key := tokenKey(mapping.Pattern, mapping.Kind)
		tokens[key] = mapping
	}

	c.version = data.Version
	c.commands = commands
	c.regexCommands = regexCommands
	c.tokens = tokens

	return nil
}

// Version returns the current catalog version.
func (c *frictionCatalog) Version() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.version
}

// normalizeCommand strips the CLI name prefix and sorts flags for consistent matching.
func (c *frictionCatalog) normalizeCommand(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	parts := strings.Fields(input)
	if len(parts) == 0 {
		return ""
	}

	if parts[0] == c.cliName {
		parts = parts[1:]
	}

	if len(parts) == 0 {
		return ""
	}

	var positionals []string
	var flags []string

	for _, part := range parts {
		if strings.HasPrefix(part, "-") {
			flags = append(flags, part)
		} else {
			positionals = append(positionals, part)
		}
	}

	sort.Strings(flags)

	result := make([]string, 0, len(positionals)+len(flags))
	result = append(result, positionals...)
	result = append(result, flags...)

	return strings.Join(result, " ")
}

// tokenKey creates a lookup key for token mappings.
func tokenKey(token string, kind FailureKind) string {
	return strings.ToLower(token) + ":" + string(kind)
}
