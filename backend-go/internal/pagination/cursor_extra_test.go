package pagination

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// Tokens must be URL-safe so they go through query strings without
// encoding. base64.RawURLEncoding uses '-' and '_' instead of '+' '/'.
func TestEncode_URLSafeCharset(t *testing.T) {
	tok := Encode(Cursor{Sort: "name", Order: "ASC", Value: "x", ID: "abc-123"})
	if strings.ContainsAny(tok, "+/=") {
		t.Errorf("token uses non-URL-safe chars: %q", tok)
	}
}

// Unicode values must survive a round-trip — names can contain emoji,
// accented chars, etc.
func TestEncode_UnicodeValueRoundTrip(t *testing.T) {
	want := Cursor{Sort: "name", Order: "ASC", Value: "ñoño 🎉", ID: "00000000-0000-0000-0000-000000000001"}
	got, err := Decode(Encode(want))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != want {
		t.Errorf("unicode round-trip lost data: %+v", got)
	}
}

// Decode should reject a token whose JSON is structurally valid but
// missing the required Order field.
func TestDecode_MissingOrder(t *testing.T) {
	// Build a token with empty order to test the "ASC"/"DESC" guard.
	raw, _ := json.Marshal(map[string]string{"s": "name", "v": "x", "i": "y"})
	tok := base64.RawURLEncoding.EncodeToString(raw)
	if _, err := Decode(tok); err != ErrBadCursor {
		t.Errorf("expected ErrBadCursor for missing Order, got %v", err)
	}
}

// WhereClause SQL must use parameterised placeholders — never inject the
// value or ID into the SQL string directly. Regression check for the
// SQL-injection footgun the audit flagged.
func TestWhereClause_AlwaysParameterised(t *testing.T) {
	c := Cursor{Sort: "name", Order: "ASC", Value: "'; DROP TABLE files; --", ID: "haha"}
	frag, args := WhereClause(c, "name")
	if strings.Contains(frag, "DROP TABLE") {
		t.Errorf("WhereClause inlined hostile value: %q", frag)
	}
	if len(args) != 3 {
		t.Errorf("expected 3 placeholder args, got %d", len(args))
	}
}

// The sort-column identifier (passed by trusted handler code from a
// whitelist) is the only spot where caller-supplied text lands in the
// SQL. Document that it's the caller's job to validate it — if you ever
// thread user input here, you've created the vuln yourself.
func TestWhereClause_SortColumnEmbeddedLiterally(t *testing.T) {
	c := Cursor{Sort: "name", Order: "ASC", Value: "x", ID: "y"}
	frag, _ := WhereClause(c, "size_bytes")
	if !strings.Contains(frag, "size_bytes >") {
		t.Errorf("expected size_bytes column in fragment, got %q", frag)
	}
}

// ErrBadCursor is the sentinel handlers check via direct equality.
// Document that any decode failure returns it (not a wrapped variant).
func TestErrBadCursor_DirectEquality(t *testing.T) {
	_, err := Decode("!!! not base64 !!!")
	if err != ErrBadCursor {
		t.Errorf("expected ErrBadCursor directly, got %v", err)
	}
}
