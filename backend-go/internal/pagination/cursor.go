// Package pagination provides opaque cursor-based paging for list endpoints.
//
// Why cursors instead of offset?
//
//	LIMIT ? OFFSET N forces MariaDB to read and discard the first N rows on
//	every page. For folders with 10 000+ files that turns into a full scan
//	per page-flip and dominates handler latency. Keyset pagination — "give
//	me the next 50 rows after (sort_value, id) X" — uses the existing index
//	on (user_id, folder_id, sort_field, id) and degrades like O(page_size)
//	instead of O(page * page_size).
//
// The cursor is base64url(JSON). It's *opaque* by contract: clients must
// echo the value back without parsing it. That gives us room to evolve the
// payload shape without breaking integrations.
//
// Cursors are NOT signed — the underlying query is always user-scoped, so a
// hostile client trying to inject another user's sort_value still hits the
// `user_id=?` predicate and gets nothing back. Adding a signature would
// double the cursor length without buying anything new.
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// Cursor describes a position in an ordered result set. The composite
// (Value, ID) is necessary because two rows can share a Value (e.g. files
// with the same name); the ID breaks the tie and makes the keyset query
// deterministic.
type Cursor struct {
	Sort  string `json:"s"` // e.g. "name", "size_bytes" — for validation
	Order string `json:"o"` // "ASC" or "DESC"
	Value string `json:"v"` // last row's value of the sort column (string-encoded)
	ID    string `json:"i"` // last row's UUID
}

// ErrBadCursor is returned when Decode is handed a malformed value. Handlers
// surface it as a 400 to nudge the client to drop their stale cursor.
var ErrBadCursor = errors.New("malformed cursor")

// Encode renders c into a URL-safe opaque token. Empty cursor → "" so
// pagers can emit the literal "first page" hint without a special case.
func Encode(c Cursor) string {
	if c.ID == "" {
		return ""
	}
	payload, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(payload)
}

// Decode parses token back into a Cursor. Returns ErrBadCursor for any kind
// of malformedness; the empty input returns a zero cursor (no error) so
// "missing cursor" and "page 1" share a code path.
func Decode(token string) (Cursor, error) {
	if token == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil {
		return Cursor{}, ErrBadCursor
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return Cursor{}, ErrBadCursor
	}
	if c.Order != "ASC" && c.Order != "DESC" {
		return Cursor{}, ErrBadCursor
	}
	return c, nil
}

// WhereClause builds the keyset SQL predicate matching c. For an ASC cursor
// the next page is "rows AFTER (value, id)" — i.e. "value > ? OR (value = ?
// AND id > ?)" — and the inverse for DESC. The caller appends this to its
// existing WHERE chain and supplies the bound parameters.
//
// Returns (sqlFragment, args). sqlFragment is empty for an empty cursor;
// callers should range/append idempotently.
//
// IMPORTANT: sortColumn must be a vetted, hard-coded identifier — never user
// input. Pass it from the same allowlist your ORDER BY uses to avoid SQL
// injection through the column name.
func WhereClause(c Cursor, sortColumn string) (string, []any) {
	if c.ID == "" {
		return "", nil
	}
	cmp := ">"
	if c.Order == "DESC" {
		cmp = "<"
	}
	// Composite tiebreaker: strictly greater on the sort field OR equal-on-
	// sort and strictly greater on id. Without the tiebreaker, pages can
	// duplicate or skip rows when adjacent rows share the sort value.
	frag := " AND (" + sortColumn + " " + cmp + " ? OR (" + sortColumn + " = ? AND id " + cmp + " ?))"
	return frag, []any{c.Value, c.Value, c.ID}
}
