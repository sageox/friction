package frictionax

// suggestionEngine chains catalog and Levenshtein suggesters to provide
// the best available correction for CLI errors.
type suggestionEngine struct {
	catalog     catalog
	levenshtein *levenshteinSuggester
}

// newSuggestionEngine creates a suggestionEngine with the given catalog
// and a default levenshteinSuggester (maxDist: 2).
func newSuggestionEngine(cat catalog) *suggestionEngine {
	return &suggestionEngine{
		catalog:     cat,
		levenshtein: newLevenshteinSuggester(2),
	}
}

// suggestForCommand attempts to find a suggestion for the given command.
func (e *suggestionEngine) suggestForCommand(fullCmd string, ctx suggestContext) *Suggestion {
	suggestion, _ := e.suggestForCommandWithMapping(fullCmd, ctx)
	return suggestion
}

// suggestForCommandWithMapping attempts to find a suggestion and returns both
// the suggestion and the original catalog mapping (if from catalog).
func (e *suggestionEngine) suggestForCommandWithMapping(fullCmd string, ctx suggestContext) (*Suggestion, *CommandMapping) {
	// try full command remap from catalog first
	if e.catalog != nil {
		if mapping := e.catalog.LookupCommand(fullCmd); mapping != nil {
			corrected, ok := mapping.ApplyMapping(fullCmd)
			if !ok {
				corrected = mapping.Target
			}
			return &Suggestion{
				Type:        SuggestionCommandRemap,
				Original:    fullCmd,
				Corrected:   corrected,
				Confidence:  mapping.Confidence,
				Description: mapping.Description,
			}, mapping
		}
	}

	// try token-level catalog lookup
	if e.catalog != nil && ctx.BadToken != "" {
		if mapping := e.catalog.LookupToken(ctx.BadToken, ctx.Kind); mapping != nil {
			return &Suggestion{
				Type:       SuggestionTokenFix,
				Original:   ctx.BadToken,
				Corrected:  mapping.Target,
				Confidence: mapping.Confidence,
			}, nil
		}
	}

	// fall back to Levenshtein
	if ctx.BadToken != "" && len(ctx.ValidOptions) > 0 {
		if suggestion := e.levenshtein.Suggest(ctx.BadToken, ctx); suggestion != nil {
			return suggestion, nil
		}
	}

	return nil, nil
}
