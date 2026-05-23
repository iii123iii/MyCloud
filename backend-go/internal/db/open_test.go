package db

import "testing"

// Open with an invalid DSN that fails at sql.Open or Ping. We don't have
// a real MySQL in CI, so this exercises the Open code path that returns
// an error early.
func TestOpen_BadDSNReturnsError(t *testing.T) {
	if _, err := Open("invalid dsn here"); err == nil {
		t.Errorf("expected error for bad DSN")
	}
}

// Open with a syntactically-OK DSN that points at no server should also
// return an error (from Ping).
func TestOpen_UnreachableHostErrors(t *testing.T) {
	// Port 1 is reserved/unused; mysql driver may still return DSN parse
	// success but Ping must fail.
	_, err := Open("user:pass@tcp(127.0.0.1:1)/db?parseTime=true&timeout=200ms")
	if err == nil {
		t.Errorf("expected ping error for unreachable host")
	}
}
