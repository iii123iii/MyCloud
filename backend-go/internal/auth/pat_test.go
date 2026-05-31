package auth

import (
	"strings"
	"testing"
)

func TestGeneratePATToken(t *testing.T) {
	full, prefix, last4, err := GeneratePATToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasPrefix(full, PATPrefix) {
		t.Errorf("token missing prefix: %q", full)
	}
	if !IsPATToken(full) {
		t.Error("IsPATToken returned false for a generated token")
	}
	if !strings.HasPrefix(full, prefix) {
		t.Errorf("prefix %q is not a prefix of %q", prefix, full)
	}
	if last4 != full[len(full)-4:] {
		t.Errorf("last_four %q does not match token tail", last4)
	}
	full2, _, _, _ := GeneratePATToken()
	if full == full2 {
		t.Error("two generated tokens collided")
	}
}

func TestHashPATToken(t *testing.T) {
	h1 := HashPATToken("mc_pat_abc")
	if len(h1) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(h1))
	}
	if HashPATToken("mc_pat_abc") != h1 {
		t.Error("hash is not deterministic")
	}
	if HashPATToken("mc_pat_xyz") == h1 {
		t.Error("distinct tokens produced the same hash")
	}
}

func TestIsPATToken(t *testing.T) {
	if IsPATToken("eyJhbGciOiJIUzI1NiJ9.payload.sig") {
		t.Error("a JWT was misidentified as a PAT")
	}
	if !IsPATToken("mc_pat_anything") {
		t.Error("a PAT was not identified")
	}
}
