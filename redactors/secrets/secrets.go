// Package secrets implements a frictionax.Redactor with built-in secret detection patterns.
//
// It provides regex-based detection and redaction for common credential types
// including AWS keys, GitHub tokens, API keys, connection strings, JWTs, and more.
// Custom patterns can be added at runtime via AddPattern.
//
// The redactor is safe for concurrent use.
package secrets

import (
	"regexp"
	"sync"
)

// Pattern defines a secret detection pattern.
type Pattern struct {
	Name    string         // identifier for the pattern
	Regex   *regexp.Regexp // compiled regex to match secrets
	Replace string         // replacement text, e.g., "[REDACTED_AWS_KEY]"
}

// Redactor detects and redacts secrets from strings.
// Implements frictionax.Redactor interface.
// Safe for concurrent use.
type Redactor struct {
	patterns []Pattern
	mu       sync.RWMutex
}

// New creates a Redactor with default secret patterns.
func New() *Redactor {
	return &Redactor{patterns: DefaultPatterns()}
}

// NewWithPatterns creates a Redactor with custom patterns.
func NewWithPatterns(patterns []Pattern) *Redactor {
	return &Redactor{patterns: patterns}
}

// Redact replaces all detected secrets in the input string.
// Implements frictionax.Redactor interface.
func (r *Redactor) Redact(input string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	output := input
	for _, p := range r.patterns {
		if p.Regex != nil {
			output = p.Regex.ReplaceAllString(output, p.Replace)
		}
	}
	return output
}

// ContainsSecrets checks if a string contains any detectable secrets.
func (r *Redactor) ContainsSecrets(input string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.patterns {
		if p.Regex != nil && p.Regex.MatchString(input) {
			return true
		}
	}
	return false
}

// AddPattern adds an additional pattern to the redactor.
func (r *Redactor) AddPattern(p Pattern) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.patterns = append(r.patterns, p)
}
