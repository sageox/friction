package frictionax

// PatternDetail represents an aggregated friction pattern with actor breakdown.
// This is the wire format for pattern aggregation APIs.
type PatternDetail struct {
	Pattern       string `json:"pattern"`
	Kind          string `json:"kind"`
	ErrorMsg      string `json:"error_msg,omitempty"`
	TotalCount    int64  `json:"total_count"`
	HumanCount    int64  `json:"human_count"`
	AgentCount    int64  `json:"agent_count"`
	AgentTypes    int64  `json:"agent_types"`
	LatestVersion string `json:"latest_version,omitempty"`
	FirstSeen     string `json:"first_seen"`
	LastSeen      string `json:"last_seen"`
}

// PatternsResponse wraps pattern details for API responses.
type PatternsResponse struct {
	Patterns []PatternDetail `json:"patterns"`
	Total    int             `json:"total"`
}
