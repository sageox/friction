// Package frictionax provides CLI friction detection, correction, and telemetry
// for any command-line tool.
//
// It detects CLI usage errors (typos, unknown commands, unknown flags) and
// provides helpful corrections with optional auto-execution for high-confidence
// matches. Designed to work with any CLI framework via pluggable adapters.
//
// # Core Concepts
//
//   - FrictionEvent: Captures a CLI usage failure for analytics
//   - Suggestion: Represents a correction suggestion with confidence score
//   - Handler: Processes CLI errors and generates suggestions
//   - Catalog: Stores learned command/token mappings for high-confidence corrections
//
// # Suggestion Chain Priority
//
// When handling a CLI error, suggestions are tried in this order:
//  1. Full command remap from catalog (highest confidence)
//  2. Token-level catalog lookup
//  3. Levenshtein distance fallback (for typos)
//
// # Auto-Execute
//
// High-confidence catalog matches with AutoExecute=true can be automatically
// executed without user confirmation. This enables "desire path" support where
// common patterns are seamlessly corrected.
//
// # Actor Detection
//
// The system distinguishes between human and agent actors to:
//   - Format output appropriately (text vs JSON)
//   - Track analytics by actor type
//   - Adjust behavior based on actor patterns
//
// Actor detection is pluggable via the ActorDetector interface. The default
// EnvActorDetector checks CI environment variables, but consumers can inject
// their own detection logic (e.g., checking for specific agent frameworks).
//
// # Redaction
//
// Input redaction is pluggable via the Redactor interface. Consumers provide
// their own redaction logic for secrets and sensitive data. A NoOpRedactor is
// included for cases where redaction is not needed.
//
// # Privacy Guarantees
//
//   - Secrets are redacted via the pluggable Redactor interface
//   - File paths are bucketed to categories, not captured
//   - Error messages are truncated and sanitized
//   - No user identity or repository names captured
//
// # Auto-Execute Philosophy
//
// Not every correction is auto-executed. Only curated catalog entries with:
//   - auto_execute: true flag set
//   - Confidence >= 0.85 threshold
//   - Safe, non-destructive operations
//
// Levenshtein suggestions are NEVER auto-executed (they're typo guesses, not
// expressions of intent).
//
// # Teaching Pattern
//
// When a command is corrected, the correction is emitted in stdout (not stderr)
// so agents see it in their context and learn for subsequent calls.
//
// # Desire Paths
//
// A "desire path" is a reasonable expectation that doesn't match current behavior.
// When many users/agents make the same "mistake," that's a signal the CLI should
// work that way. This package surfaces these patterns through analytics.
//
// Created by SageOx (https://sageox.ai).
package frictionax
