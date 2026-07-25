package main

import (
	"strings"
	"unicode"
)

// normalize folds full-width ASCII to half-width, katakana to hiragana,
// drops spaces/punctuation/symbols, and lowercases — a lightweight
// normalization for comparing quiz answers.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0xFF01 && r <= 0xFF5E { // full-width ASCII -> half-width
			r -= 0xFEE0
		}
		if r >= 0x30A1 && r <= 0x30F6 { // katakana -> hiragana
			r -= 0x60
		}
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// judge returns true when the normalized correct answer appears in the
// normalized reply (substring match after normalization).
func judge(reply, correct string) bool {
	nc := normalize(correct)
	if nc == "" {
		return false
	}
	return strings.Contains(normalize(reply), nc)
}
