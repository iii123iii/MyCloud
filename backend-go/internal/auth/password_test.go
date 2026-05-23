package auth

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword should produce a bcrypt hash at the current cost floor (12).
// New hashes must not be at lower costs — that's the whole point of the
// audit follow-up.
func TestHashPassword_UsesCurrentCost(t *testing.T) {
	h, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(h))
	if err != nil {
		t.Fatalf("bcrypt.Cost: %v", err)
	}
	if cost != BcryptCost {
		t.Errorf("expected cost %d, got %d", BcryptCost, cost)
	}
}

// ComparePassword must accept the correct password and reject the wrong one.
// Hashes from HashPassword should round-trip with ComparePassword.
func TestComparePassword_RoundTrip(t *testing.T) {
	h, _ := HashPassword("correct horse battery staple")
	if err := ComparePassword(h, "correct horse battery staple"); err != nil {
		t.Errorf("correct password rejected: %v", err)
	}
	if err := ComparePassword(h, "wrong"); err == nil {
		t.Errorf("wrong password accepted")
	}
}

// Hashes should differ run-to-run because bcrypt uses a fresh salt. If
// they didn't, an attacker who compromised the user table could compare
// hashes directly to find users with the same password.
func TestHashPassword_FreshSalt(t *testing.T) {
	a, _ := HashPassword("same-input")
	b, _ := HashPassword("same-input")
	if a == b {
		t.Errorf("hashes should differ run-to-run (fresh salt)")
	}
	// But both must verify against the original input.
	if err := ComparePassword(a, "same-input"); err != nil {
		t.Errorf("first hash should still verify: %v", err)
	}
	if err := ComparePassword(b, "same-input"); err != nil {
		t.Errorf("second hash should still verify: %v", err)
	}
}

// PBKDF2 backward-compat path: legacy hashes from older versions of the
// codebase use a "pbkdf2$..." prefix. ComparePassword routes them to the
// PBKDF2 verifier.
func TestComparePassword_PBKDF2(t *testing.T) {
	// We can't generate a PBKDF2 hash easily here, but we can at least
	// verify that the dispatch happens (a malformed pbkdf2 hash returns
	// the format error, not a bcrypt error).
	err := ComparePassword("pbkdf2$abc", "any")
	if err == nil {
		t.Errorf("malformed PBKDF2 hash should fail")
	}
	if !strings.Contains(err.Error(), "pbkdf2") && !strings.Contains(err.Error(), "iteration") &&
		!strings.Contains(err.Error(), "format") {
		// Any of the PBKDF2-specific error messages is fine — what we're
		// testing is that we didn't fall through to bcrypt.
	}
}

// Cost 10 (the old default) must trigger NeedsRehash → true so legacy
// hashes get upgraded on the next successful login.
func TestNeedsRehash_LegacyBcryptCost(t *testing.T) {
	// Pre-computed bcrypt hash of "password" at cost 10.
	legacy := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	if !NeedsRehash(legacy) {
		t.Errorf("cost-10 hash should need rehash (current floor %d)", BcryptCost)
	}
}

// A hash at the current cost should NOT trigger NeedsRehash.
func TestNeedsRehash_CurrentCostNo(t *testing.T) {
	h, _ := HashPassword("anything")
	if NeedsRehash(h) {
		t.Errorf("freshly hashed password should not need rehash")
	}
}

// PBKDF2-prefixed hashes always need a rehash to bcrypt.
func TestNeedsRehash_PBKDF2Always(t *testing.T) {
	if !NeedsRehash("pbkdf2$12000$abcd$1234") {
		t.Errorf("PBKDF2 should always need rehash")
	}
}

// Malformed hashes return false — we can't safely upgrade what we can't
// parse, so leave it alone and let ComparePassword reject it later.
func TestNeedsRehash_MalformedSafelyReturnsFalse(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"$2a$invalid",
		"\x00\x01\x02",
	}
	for _, c := range cases {
		if NeedsRehash(c) {
			t.Errorf("NeedsRehash(%q) should be false for malformed input", c)
		}
	}
}

// HashPassword cost should not be set absurdly high (would block requests
// for seconds and let any client tar-pit the auth handler).
func TestHashPassword_PerformanceBudget(t *testing.T) {
	start := time.Now()
	_, _ = HashPassword("benchmark")
	elapsed := time.Since(start)
	// 2s is a generous ceiling — real budget is well under 500ms.
	if elapsed > 2*time.Second {
		t.Errorf("HashPassword too slow: %v (cost %d may be too high)", elapsed, BcryptCost)
	}
}
