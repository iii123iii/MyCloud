// Package dsl parses user-supplied search queries with optional field
// operators and composes safe parameterised SQL fragments.
//
// Examples of supported queries:
//
//	report                                  → plain text search
//	name:report.pdf                          → exact field match
//	name:*.pdf size:>1mb                     → glob + comparator
//	tag:invoice after:2025-01-01             → tag join + date filter
//	mime:image/* owner:me                    → mime prefix + self alias
//	"product launch" size:<5mb               → quoted phrase + numeric op
//
// Supported fields:
//
//	name      string  ops: ":" (LIKE %x% if has wildcard, else exact)
//	mime      string  ops: ":" (LIKE prefix when ends in /*)
//	tag       string  ops: ":" (joins tags table)
//	size      number  ops: ":>", ":<", ":>=", ":<=", ":" (=); suffix: b/kb/mb/gb
//	after     date    ops: ":" (created_at >= date)
//	before    date    ops: ":" (created_at <= date)
//	owner     string  ops: ":" (special value "me" → current user)
//	starred   bool    ops: ":" (true/false)
//
// Tokens without a known field prefix join the free-text branch.
//
// SECURITY: The composer NEVER interpolates user input into SQL strings.
// Column names come from a hard-coded allowlist; values are always bound
// parameters. A test suite exercises adversarial inputs.
package dsl

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Query is the parsed form: a list of terms (free-text) plus structured
// filters. The composer translates this into SQL.
type Query struct {
	Terms      []string  // unstructured text — applied against name + content
	Name       []match   // explicit name: filters
	Mime       []match   // mime: filters (supports type/*)
	Tags       []string  // tag: filters (AND across multiple)
	Sizes      []sizeOp  // size: with comparator
	After      time.Time // after: lower bound on created_at
	Before     time.Time // before: upper bound on created_at
	OwnerSelf  bool      // owner:me
	StarredSet bool
	Starred    bool
}

type match struct {
	Value  string
	Prefix bool // for mime:image/* style
	Glob   bool // for name:*.pdf — translates to LIKE
}

type sizeOp struct {
	Op    string // "=", ">", "<", ">=", "<="
	Bytes int64
}

// Parse splits the input on whitespace (honouring quoted phrases) and
// classifies each token. Unknown field prefixes fall back to text.
func Parse(input string) (Query, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return Query{}, err
	}
	q := Query{}
	for _, tok := range tokens {
		field, rawVal, op, isField := splitFieldOp(tok)
		if !isField {
			q.Terms = append(q.Terms, tok)
			continue
		}
		val := strings.Trim(rawVal, `"`)
		switch strings.ToLower(field) {
		case "name":
			q.Name = append(q.Name, matchFromText(val))
		case "mime":
			m := matchFromText(val)
			if strings.HasSuffix(val, "/*") {
				m.Prefix = true
				m.Value = strings.TrimSuffix(val, "/*")
				m.Glob = false
			}
			q.Mime = append(q.Mime, m)
		case "tag":
			q.Tags = append(q.Tags, val)
		case "size":
			b, err := parseSize(val)
			if err != nil {
				return Query{}, fmt.Errorf("size: %w", err)
			}
			cmp := op
			if cmp == "" || cmp == "=" {
				cmp = "="
			}
			q.Sizes = append(q.Sizes, sizeOp{Op: cmp, Bytes: b})
		case "after":
			t, err := parseDate(val)
			if err != nil {
				return Query{}, fmt.Errorf("after: %w", err)
			}
			q.After = t
		case "before":
			t, err := parseDate(val)
			if err != nil {
				return Query{}, fmt.Errorf("before: %w", err)
			}
			q.Before = t
		case "owner":
			if strings.EqualFold(val, "me") {
				q.OwnerSelf = true
			}
		case "starred":
			q.StarredSet = true
			q.Starred = strings.EqualFold(val, "true") || val == "1"
		default:
			q.Terms = append(q.Terms, tok) // unknown field — treat as text
		}
	}
	return q, nil
}

// Compile turns q into a SQL fragment plus the bound args. The fragment is
// designed to be appended to a base WHERE that already filters by user_id
// and is_deleted=0; it always starts with " AND ...".
//
// userID is required when q.OwnerSelf is true — the composer doesn't read
// the calling context.
func Compile(q Query, userID string) (string, []any) {
	var parts []string
	var args []any

	for _, t := range q.Terms {
		// Free-text matches BOTH file name and (where indexed) content.
		// We use LIKE rather than MATCH because the composer doesn't know
		// whether the FULLTEXT index is reachable for this WHERE shape.
		parts = append(parts, "(name LIKE ? OR id IN (SELECT file_id FROM file_text WHERE content LIKE ?))")
		args = append(args, "%"+t+"%", "%"+t+"%")
	}
	for _, m := range q.Name {
		if m.Glob {
			parts = append(parts, "name LIKE ?")
			args = append(args, sqlGlob(m.Value))
		} else {
			parts = append(parts, "name = ?")
			args = append(args, m.Value)
		}
	}
	for _, m := range q.Mime {
		if m.Prefix {
			parts = append(parts, "mime_type LIKE ?")
			args = append(args, m.Value+"/%")
		} else {
			parts = append(parts, "mime_type = ?")
			args = append(args, m.Value)
		}
	}
	for _, tg := range q.Tags {
		// Each tag is an EXISTS subquery so multiple tag:X tag:Y means AND.
		parts = append(parts, `EXISTS (
			SELECT 1 FROM file_tags ft JOIN tags t ON t.id = ft.tag_id
			WHERE ft.file_id = files.id AND t.name = ?
		)`)
		args = append(args, tg)
	}
	for _, s := range q.Sizes {
		parts = append(parts, "size_bytes "+s.Op+" ?")
		args = append(args, s.Bytes)
	}
	if !q.After.IsZero() {
		parts = append(parts, "created_at >= ?")
		args = append(args, q.After)
	}
	if !q.Before.IsZero() {
		parts = append(parts, "created_at <= ?")
		args = append(args, q.Before)
	}
	if q.OwnerSelf {
		parts = append(parts, "user_id = ?")
		args = append(args, userID)
	}
	if q.StarredSet {
		parts = append(parts, "is_starred = ?")
		args = append(args, q.Starred)
	}
	if len(parts) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(parts, " AND "), args
}

// matchFromText turns "*.pdf" into a Glob match. Wildcards: * → SQL %.
func matchFromText(s string) match {
	if strings.ContainsAny(s, "*?") {
		return match{Value: s, Glob: true}
	}
	return match{Value: s}
}

// sqlGlob translates user-style wildcards (*, ?) to SQL LIKE (%, _).
// LIKE special chars (%, _) in the user value are escaped so a literal
// underscore in a filename isn't accidentally treated as a single-char wildcard.
func sqlGlob(g string) string {
	var b strings.Builder
	for _, r := range g {
		switch r {
		case '*':
			b.WriteByte('%')
		case '?':
			b.WriteByte('_')
		case '%', '_':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// tokenize splits on whitespace honouring double-quoted phrases. A bare "
// at start/end of a token is preserved so the caller's field/op split can
// see "field:value with space" via field:"value with space".
func tokenize(input string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range input {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case unicodeIsSpace(r) && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if inQuote {
		return nil, errors.New("unmatched quote")
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out, nil
}

// splitFieldOp picks "field:value" or "field:>value" out of a token.
// Returns (field, value, op, true) when the token looks like a field;
// (token, "", "", false) otherwise.
func splitFieldOp(tok string) (string, string, string, bool) {
	colon := strings.IndexByte(tok, ':')
	if colon <= 0 || colon == len(tok)-1 {
		return tok, "", "", false
	}
	field := tok[:colon]
	rest := tok[colon+1:]
	// Bare value — also support comparator chars right after colon:
	// size:>1mb → op=">"
	op := ""
	if len(rest) >= 2 {
		switch rest[:2] {
		case ">=", "<=":
			op = rest[:2]
			rest = rest[2:]
		}
	}
	if op == "" && len(rest) >= 1 {
		switch rest[0] {
		case '>', '<':
			op = string(rest[0])
			rest = rest[1:]
		}
	}
	return field, rest, op, true
}

func unicodeIsSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// parseSize accepts "1024", "5kb", "1.5mb", "2gb". Case-insensitive.
func parseSize(s string) (int64, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, errors.New("empty size")
	}
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "gb"):
		mult = 1 << 30
		s = strings.TrimSuffix(s, "gb")
	case strings.HasSuffix(s, "mb"):
		mult = 1 << 20
		s = strings.TrimSuffix(s, "mb")
	case strings.HasSuffix(s, "kb"):
		mult = 1 << 10
		s = strings.TrimSuffix(s, "kb")
	case strings.HasSuffix(s, "b"):
		s = strings.TrimSuffix(s, "b")
	}
	s = strings.TrimSpace(s)
	if strings.Contains(s, ".") {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, err
		}
		return int64(f * float64(mult)), nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return n * mult, nil
}

// parseDate accepts YYYY-MM-DD or full RFC3339.
func parseDate(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
