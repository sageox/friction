package frictionax

import "strings"

// BuildConfig controls the catalog build process.
type BuildConfig struct {
	MinHumanCount int
	MinAgentCount int
	MinTotalCount int
	SkipKinds     []string
	DiffOnly      bool
}

// DefaultBuildConfig returns sensible defaults for catalog building.
func DefaultBuildConfig() BuildConfig {
	return BuildConfig{
		MinHumanCount: 2,
		MinAgentCount: 3,
		MinTotalCount: 2,
		SkipKinds:     []string{string(FailureUnknownFlag)},
	}
}

// BuildResult contains the output of a catalog build.
type BuildResult struct {
	Catalog    CatalogData      `json:"catalog"`
	NewEntries []CommandMapping `json:"new_entries"`
	Skipped    []SkippedPattern `json:"skipped"`
}

// SkippedPattern records why a pattern was not added to the catalog.
type SkippedPattern struct {
	Pattern string `json:"pattern"`
	Kind    string `json:"kind"`
	Reason  string `json:"reason"`
}

// Build creates catalog entries from pattern data, deduplicating against an existing catalog.
// It filters patterns by thresholds, skips configured kinds, and produces CommandMapping
// entries with empty Target fields (targets are determined by LLM reasoning in the skill layer).
func Build(patterns []PatternDetail, existing CatalogData, cfg BuildConfig) (*BuildResult, error) {
	existingPatterns := make(map[string]bool, len(existing.Commands))
	for _, cmd := range existing.Commands {
		existingPatterns[strings.ToLower(cmd.Pattern)] = true
	}

	skipKinds := make(map[string]bool, len(cfg.SkipKinds))
	for _, k := range cfg.SkipKinds {
		skipKinds[k] = true
	}

	result := &BuildResult{
		Catalog: CatalogData{
			Version:  existing.Version,
			Commands: make([]CommandMapping, len(existing.Commands)),
			Tokens:   existing.Tokens,
		},
		NewEntries: []CommandMapping{},
		Skipped:    []SkippedPattern{},
	}
	copy(result.Catalog.Commands, existing.Commands)

	for _, p := range patterns {
		normalized := strings.ToLower(strings.TrimSpace(p.Pattern))
		if normalized == "" {
			continue
		}

		if skipKinds[p.Kind] {
			result.Skipped = append(result.Skipped, SkippedPattern{
				Pattern: p.Pattern, Kind: p.Kind, Reason: "skipped-kind",
			})
			continue
		}

		if existingPatterns[normalized] {
			result.Skipped = append(result.Skipped, SkippedPattern{
				Pattern: p.Pattern, Kind: p.Kind, Reason: "already-in-catalog",
			})
			continue
		}

		// pattern qualifies if it meets total threshold AND at least one actor threshold
		meetsHuman := cfg.MinHumanCount > 0 && p.HumanCount >= int64(cfg.MinHumanCount)
		meetsAgent := cfg.MinAgentCount > 0 && p.AgentCount >= int64(cfg.MinAgentCount)
		meetsTotal := p.TotalCount >= int64(cfg.MinTotalCount)

		if !meetsTotal || (!meetsHuman && !meetsAgent) {
			result.Skipped = append(result.Skipped, SkippedPattern{
				Pattern: p.Pattern, Kind: p.Kind, Reason: "below-threshold",
			})
			continue
		}

		entry := CommandMapping{
			Pattern: p.Pattern,
			Count:   int(p.TotalCount),
		}

		result.NewEntries = append(result.NewEntries, entry)
		if !cfg.DiffOnly {
			result.Catalog.Commands = append(result.Catalog.Commands, entry)
		}

		existingPatterns[normalized] = true
	}

	if cfg.DiffOnly {
		result.Catalog.Commands = result.NewEntries
	}

	return result, nil
}
