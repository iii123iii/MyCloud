package app

import (
	"strings"
	"testing"
)

// inClause builds a "(?,?,?)" placeholder string + args slice from a list
// of string IDs. Used for `WHERE id IN (?,?,?)` queries. Critical that we
// keep this parameterised — never inline the IDs.

func TestInClause_SingleID(t *testing.T) {
	ph, args := inClause([]string{"a"})
	if ph != "(?)" {
		t.Errorf("got %q, want (?)", ph)
	}
	if len(args) != 1 || args[0] != "a" {
		t.Errorf("args wrong: %v", args)
	}
}

func TestInClause_MultipleIDs(t *testing.T) {
	ph, args := inClause([]string{"a", "b", "c"})
	if ph != "(?,?,?)" {
		t.Errorf("got %q, want (?,?,?)", ph)
	}
	if len(args) != 3 || args[0] != "a" || args[1] != "b" || args[2] != "c" {
		t.Errorf("args wrong: %v", args)
	}
}

func TestInClause_Many(t *testing.T) {
	n := 100
	ids := make([]string, n)
	for i := range ids {
		ids[i] = "x"
	}
	ph, args := inClause(ids)
	if strings.Count(ph, "?") != n {
		t.Errorf("expected %d placeholders, got %d", n, strings.Count(ph, "?"))
	}
	if len(args) != n {
		t.Errorf("expected %d args, got %d", n, len(args))
	}
}

func TestInClause_HostileIDs_NotInlined(t *testing.T) {
	// SQL-injection attempt as an ID. The whole point of placeholders:
	// the value never lands in the SQL string.
	ph, args := inClause([]string{"'; DROP TABLE files; --"})
	if strings.Contains(ph, "DROP") {
		t.Errorf("inClause inlined hostile input into SQL: %q", ph)
	}
	if len(args) != 1 {
		t.Errorf("expected 1 arg")
	}
}
