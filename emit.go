package frictionax

// CRITICAL: HUMAN REVIEW REQUIRED
// The emit formats below are intentionally designed for agent learning.
// Changes to these structures can break agent parsing and learning behavior.

// correctionMeta contains metadata about a command correction that was auto-executed.
type correctionMeta struct {
	Was  string `json:"was"`
	Now  string `json:"now"`
	Note string `json:"note"`
}

// suggestionMeta contains metadata about a command suggestion (not auto-executed).
type suggestionMeta struct {
	Try  string `json:"try"`
	Note string `json:"note"`
}

// correctionOutput wraps command output with correction/suggestion metadata.
type correctionOutput struct {
	Corrected  *correctionMeta `json:"_corrected,omitempty"`
	Suggestion *suggestionMeta `json:"_suggestion,omitempty"`
	Result     any             `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
}
