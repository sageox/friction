//nolint:misspell // intentional misspellings in test data for Levenshtein distance testing
package levenshtein

import (
	"testing"
)

func TestDistance(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected int
	}{
		// same strings = 0
		{name: "identical empty strings", a: "", b: "", expected: 0},
		{name: "identical single char", a: "a", b: "a", expected: 0},
		{name: "identical word", a: "hello", b: "hello", expected: 0},
		{name: "identical long string", a: "levenshtein", b: "levenshtein", expected: 0},

		// empty string cases
		{name: "empty to single char", a: "", b: "a", expected: 1},
		{name: "single char to empty", a: "a", b: "", expected: 1},
		{name: "empty to word", a: "", b: "hello", expected: 5},
		{name: "word to empty", a: "world", b: "", expected: 5},

		// single character differences
		{name: "single substitution at start", a: "cat", b: "bat", expected: 1},
		{name: "single substitution at end", a: "cat", b: "car", expected: 1},
		{name: "single substitution in middle", a: "cat", b: "cot", expected: 1},
		{name: "single insertion", a: "cat", b: "cats", expected: 1},
		{name: "single deletion", a: "cats", b: "cat", expected: 1},
		{name: "single insertion at start", a: "at", b: "cat", expected: 1},
		{name: "single deletion at start", a: "cat", b: "at", expected: 1},

		// transpositions (adjacent swaps - costs 2 in standard Levenshtein)
		{name: "adjacent transposition ab->ba", a: "ab", b: "ba", expected: 2},
		{name: "transposition in word", a: "cat", b: "act", expected: 2},
		{name: "transposition teh->the", a: "teh", b: "the", expected: 2},

		// multiple edits
		{name: "two substitutions", a: "cat", b: "dog", expected: 3},
		{name: "kitten to sitting", a: "kitten", b: "sitting", expected: 3},
		{name: "saturday to sunday", a: "saturday", b: "sunday", expected: 3},
		{name: "flaw to lawn", a: "flaw", b: "lawn", expected: 2},

		// CLI typos (realistic use cases)
		{name: "typo: stauts -> status", a: "stauts", b: "status", expected: 2},
		{name: "typo: hepl -> help", a: "hepl", b: "help", expected: 2},
		{name: "typo: agnet -> agent", a: "agnet", b: "agent", expected: 2},
		{name: "typo: cofnig -> config", a: "cofnig", b: "config", expected: 2},
		{name: "typo: inital -> initial", a: "inital", b: "initial", expected: 1},
		{name: "typo: transcirpt -> transcript", a: "transcirpt", b: "transcript", expected: 2},

		// unicode support
		{name: "unicode identical", a: "日本語", b: "日本語", expected: 0},
		{name: "unicode single substitution", a: "日本語", b: "日本人", expected: 1},
		{name: "unicode different length", a: "日本", b: "日本語", expected: 1},
		{name: "unicode to ascii", a: "cafe", b: "café", expected: 1},
		{name: "emoji identical", a: "hello\U0001F600", b: "hello\U0001F600", expected: 0},
		{name: "emoji substitution", a: "hi\U0001F600", b: "hi\U0001F601", expected: 1},
		{name: "mixed unicode", a: "hello世界", b: "hello世界!", expected: 1},

		// case sensitivity
		{name: "case difference single", a: "A", b: "a", expected: 1},
		{name: "case difference word", a: "Hello", b: "hello", expected: 1},
		{name: "case difference all caps", a: "HELLO", b: "hello", expected: 5},

		// edge cases with repeated chars
		{name: "repeated to single", a: "aaa", b: "a", expected: 2},
		{name: "single to repeated", a: "a", b: "aaa", expected: 2},
		{name: "different repeated", a: "aaa", b: "bbb", expected: 3},

		// completely different
		{name: "completely different same length", a: "abc", b: "xyz", expected: 3},
		{name: "completely different diff length", a: "abc", b: "wxyz", expected: 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Distance(tc.a, tc.b)
			if got != tc.expected {
				t.Errorf("Distance(%q, %q) = %d; want %d", tc.a, tc.b, got, tc.expected)
			}
		})
	}
}

func TestDistance_Symmetry(t *testing.T) {
	pairs := [][2]string{
		{"", "abc"},
		{"hello", "world"},
		{"kitten", "sitting"},
		{"日本語", "中文"},
		{"cafe", "café"},
		{"agent", "agnet"},
	}

	for _, pair := range pairs {
		a, b := pair[0], pair[1]
		t.Run(a+"_"+b, func(t *testing.T) {
			distAB := Distance(a, b)
			distBA := Distance(b, a)
			if distAB != distBA {
				t.Errorf("symmetry violation: distance(%q, %q)=%d != distance(%q, %q)=%d",
					a, b, distAB, b, a, distBA)
			}
		})
	}
}

func TestDistance_TriangleInequality(t *testing.T) {
	triples := [][3]string{
		{"cat", "bat", "bar"},
		{"hello", "hallo", "halli"},
		{"", "a", "ab"},
		{"kitten", "mitten", "sitting"},
		{"agent", "agnet", "agent"},
	}

	for _, triple := range triples {
		a, b, c := triple[0], triple[1], triple[2]
		t.Run(a+"_"+b+"_"+c, func(t *testing.T) {
			distAC := Distance(a, c)
			distAB := Distance(a, b)
			distBC := Distance(b, c)
			if distAC > distAB+distBC {
				t.Errorf("triangle inequality violation: distance(%q, %q)=%d > distance(%q, %q)=%d + distance(%q, %q)=%d",
					a, c, distAC, a, b, distAB, b, c, distBC)
			}
		})
	}
}

func TestDistance_IdentityOfIndiscernibles(t *testing.T) {
	strs := []string{"", "a", "hello", "日本語", "cafe", "café"}

	for _, s := range strs {
		t.Run("self_"+s, func(t *testing.T) {
			dist := Distance(s, s)
			if dist != 0 {
				t.Errorf("identity violation: distance(%q, %q) = %d; want 0", s, s, dist)
			}
		})
	}

	pairs := [][2]string{
		{"a", "b"},
		{"hello", "hallo"},
		{"", "x"},
	}
	for _, pair := range pairs {
		a, b := pair[0], pair[1]
		t.Run("non_self_"+a+"_"+b, func(t *testing.T) {
			dist := Distance(a, b)
			if dist == 0 {
				t.Errorf("identity violation: distance(%q, %q) = 0 but strings are different", a, b)
			}
		})
	}
}

func TestDistance_BoundedByMaxLength(t *testing.T) {
	pairs := [][2]string{
		{"", "abcdef"},
		{"abc", "xyz"},
		{"hello", "world"},
		{"a", "abcdefghij"},
		{"日本", "abc"},
	}

	for _, pair := range pairs {
		a, b := pair[0], pair[1]
		t.Run(a+"_"+b, func(t *testing.T) {
			dist := Distance(a, b)
			maxLen := len([]rune(a))
			if len([]rune(b)) > maxLen {
				maxLen = len([]rune(b))
			}
			if dist > maxLen {
				t.Errorf("bound violation: distance(%q, %q)=%d > max_rune_length=%d",
					a, b, dist, maxLen)
			}
		})
	}
}

func BenchmarkDistance_Short(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Distance("kitten", "sitting")
	}
}

func BenchmarkDistance_Medium(b *testing.B) {
	a := "levenshtein"
	c := "frankenstein"
	for i := 0; i < b.N; i++ {
		Distance(a, c)
	}
}

func BenchmarkDistance_Long(b *testing.B) {
	a := "the quick brown fox jumps over the lazy dog"
	c := "the lazy dog jumps over the quick brown fox"
	for i := 0; i < b.N; i++ {
		Distance(a, c)
	}
}

func BenchmarkDistance_Unicode(b *testing.B) {
	a := "日本語テキスト"
	c := "中文字テスト"
	for i := 0; i < b.N; i++ {
		Distance(a, c)
	}
}
