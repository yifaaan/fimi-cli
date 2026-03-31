package completer

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// FuzzyMatch reports whether query matches candidate as a case-insensitive
// character subsequence. It returns the match and a score for ranking.
//
// Scoring:
//   - Exact prefix match: +100
//   - Each consecutive matched pair: +2
//   - Each match in the first quarter of candidate: +5
func FuzzyMatch(query, candidate string) (matched bool, score int) {
	if query == "" {
		return true, 0
	}

	q := strings.ToLower(query)
	c := strings.ToLower(candidate)
	qLen := len(q)
	cLen := len(c)

	quarterLen := cLen / 4
	if quarterLen == 0 {
		quarterLen = 1
	}

	qi := 0
	prevMatchIdx := -1
	score = 0

	for ci := 0; ci < cLen && qi < qLen; ci++ {
		if c[ci] == q[qi] {
			if qi == 0 && ci == 0 {
				score += 100
			}
			if prevMatchIdx >= 0 && ci == prevMatchIdx+1 {
				score += 2
			}
			if ci < quarterLen {
				score += 5
			}
			score += 1
			prevMatchIdx = ci
			qi++
		}
	}

	if qi != qLen {
		return false, 0
	}
	return true, score
}

// FilterAndRank returns candidates matching query via fuzzy matching,
// sorted by descending score, limited to at most limit results.
func FilterAndRank(query string, candidates []string, limit int) []string {
	if query == "" {
		n := len(candidates)
		if n > limit {
			n = limit
		}
		result := make([]string, n)
		copy(result, candidates[:n])
		return result
	}

	type scored struct {
		candidate string
		score     int
	}

	var matches []scored
	for _, c := range candidates {
		if len(strings.TrimRight(c, "/")) == 0 {
			continue
		}
		if ok, s := FuzzyMatch(query, c); ok {
			matches = append(matches, scored{c, s})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].candidate < matches[j].candidate
	})

	n := len(matches)
	if n > limit {
		n = limit
	}

	result := make([]string, n)
	for i := range result {
		result[i] = matches[i].candidate
	}
	return result
}

// ExtractFragment extracts the text after the last '@' in the input
// before cursorPos, applying trigger guards. Returns the fragment
// and the byte position of '@'.
func ExtractFragment(text string, cursorPos int) (fragment string, atPos int, ok bool) {
	if cursorPos > len(text) {
		cursorPos = len(text)
	}

	idx := strings.LastIndex(text[:cursorPos], "@")
	if idx == -1 {
		return "", -1, false
	}

	// Guard: character before '@' must not be alphanumeric or a trigger guard
	if idx > 0 {
		prevRune, _ := utf8.DecodeLastRuneInString(text[:idx])
		if prevRune != utf8.RuneError && (unicode.IsLetter(prevRune) || unicode.IsDigit(prevRune) || isTriggerGuard(prevRune)) {
			return "", -1, false
		}
	}

	fragment = text[idx+1 : cursorPos]

	for _, r := range fragment {
		if unicode.IsSpace(r) {
			return "", -1, false
		}
	}

	return fragment, idx, true
}

var triggerGuards = map[rune]bool{
	'.': true, '-': true, '_': true,
	'`': true, '\'': true, '"': true,
	':': true, '@': true, '#': true, '~': true,
}

func isTriggerGuard(r rune) bool {
	return triggerGuards[r]
}
