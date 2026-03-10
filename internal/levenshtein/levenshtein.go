// Package levenshtein provides edit distance calculation between strings.
package levenshtein

// Distance calculates the edit distance between two strings using dynamic
// programming. Returns the minimum number of single-character edits
// (insertions, deletions, substitutions) needed to transform a into b.
func Distance(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// convert to runes for proper Unicode handling
	runesA := []rune(a)
	runesB := []rune(b)
	lenA := len(runesA)
	lenB := len(runesB)

	// use single row optimization to reduce memory from O(m*n) to O(min(m,n))
	if lenA < lenB {
		runesA, runesB = runesB, runesA
		lenA, lenB = lenB, lenA
	}

	prev := make([]int, lenB+1)
	curr := make([]int, lenB+1)

	for j := 0; j <= lenB; j++ {
		prev[j] = j
	}

	for i := 1; i <= lenA; i++ {
		curr[0] = i
		for j := 1; j <= lenB; j++ {
			cost := 1
			if runesA[i-1] == runesB[j-1] {
				cost = 0
			}
			curr[j] = min3(
				prev[j]+1,
				curr[j-1]+1,
				prev[j-1]+cost,
			)
		}
		prev, curr = curr, prev
	}

	return prev[lenB]
}

func min3(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}
