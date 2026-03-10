package frictionax

import "github.com/sageox/frictionax/internal/levenshtein"

// levenshteinSuggester suggests corrections based on edit distance.
type levenshteinSuggester struct {
	maxDistance int
}

// newLevenshteinSuggester creates a suggester that only suggests matches
// within maxDist edits.
func newLevenshteinSuggester(maxDist int) *levenshteinSuggester {
	return &levenshteinSuggester{
		maxDistance: maxDist,
	}
}

// Suggest finds the closest match to input from ctx.ValidOptions.
func (s *levenshteinSuggester) Suggest(input string, ctx suggestContext) *Suggestion {
	if len(ctx.ValidOptions) == 0 {
		return nil
	}

	bestMatch := ""
	bestDistance := s.maxDistance + 1

	for _, option := range ctx.ValidOptions {
		dist := levenshtein.Distance(input, option)
		if dist < bestDistance {
			bestDistance = dist
			bestMatch = option
		}
	}

	if bestDistance > s.maxDistance {
		return nil
	}

	confidence := 1.0 - float64(bestDistance)/float64(s.maxDistance+1)

	return &Suggestion{
		Type:       SuggestionLevenshtein,
		Original:   input,
		Corrected:  bestMatch,
		Confidence: confidence,
	}
}
