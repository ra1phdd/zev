// Package lexical provides deterministic, local text primitives for history search.
//
// ProfileID identifies the normalization and stemming profile. Normalize applies
// Unicode NFKC, Unicode case folding, and ё-to-е normalization, then collapses
// Unicode whitespace. Cyrillic tokens use the pinned Russian Snowball stemmer;
// Latin tokens use the pinned English Snowball stemmer. Other scripts remain
// neutral surface tokens. The English stemmer deliberately does not claim
// support for suppletive forms such as go/went/gone; this is the profile's
// documented stemmer ceiling.
//
// Fuzzy expansion is bounded to MaxTokenRunes runes, at most 600 unique
// deletion keys, and CandidateLimit downstream candidates. Distance and
// Similarity operate on normalized Unicode code points, and Trigrams returns
// normalized Unicode trigrams for tokens within MaxTokenRunes.
package lexical

import (
	"strings"
	"unicode"

	"github.com/kljensen/snowball"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	// ProfileID is the stable identifier for this analyzer's derived text.
	ProfileID = "snowball-v0.10.0-nfkc-casefold-yo2e-v1"

	// MaxTokenRunes bounds fuzzy token expansion and trigram generation.
	MaxTokenRunes = 32

	// CandidateLimit is the fixed maximum number of fuzzy candidates a caller
	// should admit from this profile's local indexes.
	CandidateLimit = 256

	maxDeletionKeys = 600
)

var unicodeCaseFold = cases.Fold()

// Normalize returns text in the deterministic ProfileID normalization form.
// It collapses consecutive Unicode whitespace and trims whitespace at both
// ends; punctuation is preserved for callers that need the normalized text.
func Normalize(text string) string {
	text = norm.NFKC.String(text)
	text = unicodeCaseFold.String(text)
	text = strings.ReplaceAll(text, "ё", "е")

	var normalized strings.Builder
	normalized.Grow(len(text))
	space := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			space = normalized.Len() > 0
			continue
		}
		if space {
			normalized.WriteByte(' ')
			space = false
		}
		normalized.WriteRune(r)
	}
	return normalized.String()
}

// Tokens returns normalized letter, digit, and combining-mark runs from text.
// Punctuation and whitespace separate tokens, while tokens from scripts other
// than Cyrillic and Latin are retained as neutral surface tokens.
func Tokens(text string) []string {
	text = Normalize(text)
	var tokens []string
	var token []rune
	flush := func() {
		if len(token) == 0 {
			return
		}
		tokens = append(tokens, string(token))
		token = token[:0]
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || (unicode.IsMark(r) && len(token) > 0) {
			token = append(token, r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

// Stem returns the normalized Snowball stem for one token. Cyrillic and Latin
// tokens use Russian and English respectively; all other scripts are returned
// unchanged. Snowball's English profile intentionally leaves go, went, and
// gone unrelated because irregular lemmatization is outside this stemmer's
// bounded acceptance set.
func Stem(token string) string {
	token = Normalize(token)
	language := tokenLanguage(token)
	if language == "" {
		return token
	}
	stem, err := snowball.Stem(token, language, false)
	if err != nil {
		return token
	}
	return stem
}

// Morphology returns space-separated normalized stems for all tokens in text.
func Morphology(text string) string {
	tokens := Tokens(text)
	stems := make([]string, len(tokens))
	for i, token := range tokens {
		stems[i] = Stem(token)
	}
	return strings.Join(stems, " ")
}

// DeletionKeys returns deterministic unique keys formed by deleting one or two
// Unicode code points from token. Tokens longer than MaxTokenRunes return nil.
// Empty keys are retained because deleting the only code point is a valid
// one-deletion key; callers should not admit empty-query candidates.
func DeletionKeys(token string) []string {
	runes := []rune(Normalize(token))
	if len(runes) > MaxTokenRunes {
		return nil
	}
	keys := make([]string, 0, maxDeletionKeys)
	seen := make(map[string]struct{}, maxDeletionKeys)
	add := func(key []rune) bool {
		value := string(key)
		if _, exists := seen[value]; exists {
			return true
		}
		if len(keys) == maxDeletionKeys {
			return false
		}
		seen[value] = struct{}{}
		keys = append(keys, value)
		return true
	}

	for i := range runes {
		key := make([]rune, 0, len(runes)-1)
		key = append(key, runes[:i]...)
		key = append(key, runes[i+1:]...)
		if !add(key) {
			return keys
		}
	}
	for i := range runes {
		for j := range runes {
			if j <= i {
				continue
			}
			key := make([]rune, 0, len(runes)-2)
			key = append(key, runes[:i]...)
			key = append(key, runes[i+1:j]...)
			key = append(key, runes[j+1:]...)
			if !add(key) {
				return keys
			}
		}
	}
	return keys
}

// Distance returns normalized adjacent-transposition edit distance over Unicode
// code points. An adjacent transposition costs one edit.
func Distance(a, b string) int {
	aRunes := []rune(Normalize(a))
	bRunes := []rune(Normalize(b))
	if len(aRunes) == 0 {
		return len(bRunes)
	}
	if len(bRunes) == 0 {
		return len(aRunes)
	}

	distance := make([][]int, len(aRunes)+1)
	for i := range distance {
		distance[i] = make([]int, len(bRunes)+1)
		distance[i][0] = i
	}
	for j := range distance[0] {
		distance[0][j] = j
	}
	for i, aRune := range aRunes {
		for j, bRune := range bRunes {
			cost := 0
			if aRune != bRune {
				cost = 1
			}
			distance[i+1][j+1] = min(
				distance[i][j+1]+1,
				distance[i+1][j]+1,
				distance[i][j]+cost,
			)
			if i > 0 && j > 0 && aRunes[i] == bRunes[j-1] && aRunes[i-1] == bRunes[j] {
				distance[i+1][j+1] = min(distance[i+1][j+1], distance[i-1][j-1]+1)
			}
		}
	}
	return distance[len(aRunes)][len(bRunes)]
}

// Similarity returns a normalized score in [0, 1] and whether the pair meets
// the profile's fuzzy threshold. Tokens shorter than four runes require an
// exact match; lengths four and five allow one edit; lengths six and above
// allow at most two edits and no more than 30% of the maximum token length.
func Similarity(a, b string) (float64, bool) {
	aRunes := []rune(Normalize(a))
	bRunes := []rune(Normalize(b))
	maxLength := max(len(aRunes), len(bRunes))
	distance := Distance(a, b)
	if maxLength == 0 {
		return 1, true
	}
	score := 1 - float64(distance)/float64(maxLength)
	if score < 0 {
		score = 0
	}
	if min(len(aRunes), len(bRunes)) < 4 {
		return score, distance == 0
	}
	if maxLength <= 5 {
		return score, distance <= 1
	}
	return score, distance <= 2 && distance*10 <= maxLength*3
}

// Trigrams returns ordered normalized Unicode trigrams. Tokens longer than
// MaxTokenRunes return nil to keep local fuzzy expansion bounded.
func Trigrams(token string) []string {
	runes := []rune(Normalize(token))
	if len(runes) > MaxTokenRunes || len(runes) < 3 {
		return nil
	}
	trigrams := make([]string, 0, len(runes)-2)
	for i := 0; i <= len(runes)-3; i++ {
		trigrams = append(trigrams, string(runes[i:i+3]))
	}
	return trigrams
}

func tokenLanguage(token string) string {
	var cyrillic, latin, other bool
	for _, r := range token {
		if !unicode.IsLetter(r) {
			continue
		}
		switch {
		case unicode.In(r, unicode.Cyrillic):
			cyrillic = true
		case unicode.In(r, unicode.Latin):
			latin = true
		default:
			other = true
		}
	}
	if cyrillic && !latin && !other {
		return "russian"
	}
	if latin && !cyrillic && !other {
		return "english"
	}
	return ""
}
