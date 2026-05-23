package pagination

import "testing"

func TestEncodeDecodeRoundTrip(t *testing.T) {
	want := Cursor{Sort: "name", Order: "ASC", Value: "report.pdf", ID: "abc-123"}
	tok := Encode(want)
	if tok == "" {
		t.Fatal("Encode returned empty for non-zero cursor")
	}
	got, err := Decode(tok)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != want {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, want)
	}
}

func TestEncodeEmptyReturnsEmpty(t *testing.T) {
	if got := Encode(Cursor{}); got != "" {
		t.Errorf("Encode(zero) = %q, want \"\"", got)
	}
}

func TestDecodeEmptyReturnsZero(t *testing.T) {
	got, err := Decode("")
	if err != nil {
		t.Fatalf("Decode empty: %v", err)
	}
	if got != (Cursor{}) {
		t.Errorf("Decode empty = %+v, want zero", got)
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"!!!", "not-base64", "Zm9vYmFy"} { // last is valid base64 but not JSON
		if _, err := Decode(bad); err == nil {
			t.Errorf("Decode(%q) accepted; want ErrBadCursor", bad)
		}
	}
}

func TestDecodeRejectsBadOrder(t *testing.T) {
	tok := Encode(Cursor{Sort: "name", Order: "SIDEWAYS", Value: "x", ID: "y"})
	if _, err := Decode(tok); err != ErrBadCursor {
		t.Errorf("expected ErrBadCursor for invalid order, got %v", err)
	}
}

func TestWhereClauseASC(t *testing.T) {
	c := Cursor{Sort: "name", Order: "ASC", Value: "report.pdf", ID: "abc"}
	frag, args := WhereClause(c, "name")
	wantFrag := " AND (name > ? OR (name = ? AND id > ?))"
	if frag != wantFrag {
		t.Errorf("frag = %q, want %q", frag, wantFrag)
	}
	if len(args) != 3 || args[0] != "report.pdf" || args[1] != "report.pdf" || args[2] != "abc" {
		t.Errorf("args = %v, want [report.pdf report.pdf abc]", args)
	}
}

func TestWhereClauseDESC(t *testing.T) {
	c := Cursor{Sort: "name", Order: "DESC", Value: "x", ID: "y"}
	frag, _ := WhereClause(c, "name")
	if frag != " AND (name < ? OR (name = ? AND id < ?))" {
		t.Errorf("DESC fragment wrong: %q", frag)
	}
}

func TestWhereClauseEmpty(t *testing.T) {
	frag, args := WhereClause(Cursor{}, "name")
	if frag != "" || args != nil {
		t.Errorf("empty cursor produced non-empty clause: frag=%q args=%v", frag, args)
	}
}
