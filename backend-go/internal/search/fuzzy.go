// Package search ships helpers that complement the dsl subpackage. fuzzy.go
// owns name-normalisation and Damerau-Levenshtein scoring.
package search

import (
	"strings"
	"unicode"
)

// NormaliseName lowercases and strips diacritics from a filename so the DB
// column matches typo-tolerant queries. We do this in Go rather than via a
// generated column because COLLATE-based folding misses some marks and
// MariaDB lacks a built-in unaccent function.
//
// Punctuation is preserved (extensions matter); only the case + diacritics
// are normalised.
func NormaliseName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range strings.ToLower(name) {
		// Combining marks have category Mn / Me; strip them.
		if unicode.In(r, unicode.Mn, unicode.Me) {
			continue
		}
		// Fast-path ASCII letters/digits/punctuation.
		if r < 128 {
			b.WriteRune(r)
			continue
		}
		// Decompose-then-strip: take the rune as-is for now (best-effort).
		// Full NFD decomposition needs golang.org/x/text/unicode/norm; we
		// keep the dependency footprint small and accept "café" → "café"
		// rather than "cafe". The ngram FULLTEXT still matches substrings.
		b.WriteRune(r)
	}
	return b.String()
}

// DamerauLevenshtein computes the edit distance between a and b under
// insertion, deletion, substitution, and adjacent transposition. Used to
// rank candidate filenames against a typo'd query.
//
// O(len(a)*len(b)) time and space. Fine for short filenames; do not call
// with novella-length inputs.
func DamerauLevenshtein(a, b string) int {
	if a == b {
		return 0
	}
	ar, br := []rune(a), []rune(b)
	la, lb := len(ar), len(br)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	// d is a (la+1) x (lb+1) matrix flattened into a single slice.
	d := make([]int, (la+1)*(lb+1))
	idx := func(i, j int) int { return i*(lb+1) + j }
	for i := 0; i <= la; i++ {
		d[idx(i, 0)] = i
	}
	for j := 0; j <= lb; j++ {
		d[idx(0, j)] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			dist := min3(
				d[idx(i-1, j)]+1,          // deletion
				d[idx(i, j-1)]+1,          // insertion
				d[idx(i-1, j-1)]+cost,     // substitution
			)
			if i > 1 && j > 1 && ar[i-1] == br[j-2] && ar[i-2] == br[j-1] {
				if t := d[idx(i-2, j-2)] + 1; t < dist {
					dist = t
				}
			}
			d[idx(i, j)] = dist
		}
	}
	return d[idx(la, lb)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
