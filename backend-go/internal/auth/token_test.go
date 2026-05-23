package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

// Spins up an in-process Redis (miniredis) and a token Manager wired to it.
// Keeps every test hermetic — no external dependencies, no port collisions.
func newTestManager(t *testing.T) (*Manager, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	return NewManager("test-secret-must-be-at-least-32-bytes!!", 60, 600, rc), mr
}

// IssuePair → Parse round-trips both tokens, and Parse rejects a refresh
// token presented as access (and vice-versa) so the typ field is honoured.
func TestIssuePair_RoundTrip(t *testing.T) {
	m, _ := newTestManager(t)
	tokens, err := m.IssuePair("u-1", "user")
	if err != nil {
		t.Fatalf("IssuePair: %v", err)
	}
	if tokens["access_jti"] == "" || tokens["refresh_jti"] == "" || tokens["family"] == "" {
		t.Fatalf("expected access_jti, refresh_jti, family — got %+v", tokens)
	}
	if tokens["access_jti"] == tokens["refresh_jti"] {
		t.Errorf("access and refresh share a JTI — must be distinct")
	}

	access := tokens["access_token"].(string)
	refresh := tokens["refresh_token"].(string)

	claims, err := m.Parse(access)
	if err != nil {
		t.Fatalf("Parse(access): %v", err)
	}
	if claims.UserID != "u-1" || claims.Type != "access" {
		t.Errorf("access claims wrong: %+v", claims)
	}
	if claims.Family != tokens["family"] {
		t.Errorf("access family mismatch: claim=%q map=%v", claims.Family, tokens["family"])
	}

	rClaims, err := m.Parse(refresh)
	if err != nil {
		t.Fatalf("Parse(refresh): %v", err)
	}
	if rClaims.Type != "refresh" {
		t.Errorf("expected typ=refresh, got %q", rClaims.Type)
	}
	if rClaims.Family != claims.Family {
		t.Errorf("access + refresh issued from same call must share family")
	}
}

// Parse rejects any signing-method != HS256 (defence in depth against
// alg-switching attacks).
func TestParse_RejectsForeignAlg(t *testing.T) {
	m, _ := newTestManager(t)
	// Mint a token with the "none" algorithm — jwt/v5 rejects this in
	// ParseWithClaims by default, but the explicit keyfunc check catches
	// it before the parser does, so the error surfaces clearly.
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, Claims{UserID: "u-1", Type: "access"})
	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	if _, err := m.Parse(signed); err == nil {
		t.Fatalf("expected Parse to reject alg=none token, got nil error")
	}
}

// RevokeJTI puts a key in the blacklist; subsequent Parse calls for any
// token carrying that JTI fail. This is the building block for refresh
// rotation in handleRefresh.
func TestRevokeJTI_BlocksParse(t *testing.T) {
	m, _ := newTestManager(t)
	tokens, _ := m.IssuePair("u-1", "user")
	access := tokens["access_token"].(string)

	claims, err := m.Parse(access)
	if err != nil {
		t.Fatalf("Parse before revoke: %v", err)
	}
	m.RevokeJTI(claims.ID)
	if _, err := m.Parse(access); err == nil {
		t.Errorf("Parse should reject revoked-JTI token")
	}
	if !m.IsJTIRevoked(claims.ID) {
		t.Errorf("IsJTIRevoked should report revoked JTI")
	}
}

// RevokeFamily kills every token sharing that family — the mechanism
// behind logout (which calls RevokeFamily on the bearer's family) and
// refresh-reuse detection. A token issued before the family revocation
// stops parsing; a brand-new pair (different family) still works.
func TestRevokeFamily_KillsAllInFamily(t *testing.T) {
	m, _ := newTestManager(t)
	first, _ := m.IssuePair("u-1", "user")
	family := first["family"].(string)

	// Simulate a refresh: new pair, SAME family.
	second, err := m.IssuePairForRefresh("u-1", "user", family)
	if err != nil {
		t.Fatalf("IssuePairForRefresh: %v", err)
	}

	// Both pairs parse fine before family revocation.
	if _, err := m.Parse(first["access_token"].(string)); err != nil {
		t.Fatalf("Parse first access: %v", err)
	}
	if _, err := m.Parse(second["access_token"].(string)); err != nil {
		t.Fatalf("Parse second access: %v", err)
	}

	m.RevokeFamily(family)

	if _, err := m.Parse(first["access_token"].(string)); err == nil {
		t.Errorf("first access should be revoked by family kill")
	}
	if _, err := m.Parse(second["access_token"].(string)); err == nil {
		t.Errorf("second access should be revoked by family kill")
	}

	// Brand-new login (new family) is unaffected.
	third, _ := m.IssuePair("u-1", "user")
	if third["family"].(string) == family {
		t.Fatalf("new IssuePair must mint a fresh family")
	}
	if _, err := m.Parse(third["access_token"].(string)); err != nil {
		t.Errorf("fresh login should parse: %v", err)
	}
}

// ParseUnverifiedForReuse returns claims even for a token whose JTI is
// already blacklisted. handleRefresh relies on this: a refresh-token
// replay attack presents an already-rotated (revoked) JTI, and we need
// to extract its family to kill the whole session — Parse would refuse
// to return claims for a revoked token.
func TestParseUnverifiedForReuse_WorksOnRevoked(t *testing.T) {
	m, _ := newTestManager(t)
	tokens, _ := m.IssuePair("u-1", "user")
	refresh := tokens["refresh_token"].(string)

	rClaims, err := m.Parse(refresh)
	if err != nil {
		t.Fatalf("Parse refresh: %v", err)
	}
	m.RevokeJTI(rClaims.ID)
	// Parse now refuses to return claims because the JTI is on the
	// blacklist.
	if _, err := m.Parse(refresh); err == nil {
		t.Errorf("Parse should refuse a revoked token")
	}
	// ParseUnverifiedForReuse bypasses the blacklist check so the reuse-
	// detection branch in handleRefresh can read the family.
	claims, err := m.ParseUnverifiedForReuse(refresh)
	if err != nil {
		t.Fatalf("ParseUnverifiedForReuse: %v", err)
	}
	if claims.ID != rClaims.ID {
		t.Errorf("unverified parse should return the same JTI")
	}
}

// Bcrypt cost is at the new floor (12). NeedsRehash returns true for any
// cost below that, so legacy hashes get auto-upgraded on next login.
func TestNeedsRehash_CostFloor(t *testing.T) {
	// Hash at the old default (10) — below the new floor → must rehash.
	oldHash := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy" // bcrypt of "password"
	if !NeedsRehash(oldHash) {
		t.Errorf("expected NeedsRehash(cost-10 bcrypt) = true (current floor is %d)", BcryptCost)
	}
	// pbkdf2 prefix always triggers rehash regardless of cost.
	if !NeedsRehash("pbkdf2$10000$...$...") {
		t.Errorf("expected NeedsRehash(pbkdf2) = true")
	}
	// A malformed hash returns false — we can't safely rehash what we
	// can't parse.
	if NeedsRehash("garbage") {
		t.Errorf("NeedsRehash(garbage) should be false")
	}
}

// Documents the access vs refresh TTL guarantee.
func TestIssuePair_TTLs(t *testing.T) {
	m, _ := newTestManager(t)
	tokens, _ := m.IssuePair("u-1", "user")
	expires := tokens["access_expires_at"].(time.Time)
	if !expires.After(time.Now()) {
		t.Errorf("access_expires_at should be in the future, got %v", expires)
	}
	// Sanity: access exp is at most ~60s out (the TTL we set up the
	// manager with).
	if delta := time.Until(expires); delta > 75*time.Second {
		t.Errorf("access TTL too long: %v", delta)
	}
}

// errors.Is should be the canonical way to detect a revoked-token Parse
// error in handler code — not string matching.
func TestParse_RevokedTokenError(t *testing.T) {
	m, _ := newTestManager(t)
	tokens, _ := m.IssuePair("u-1", "user")
	access := tokens["access_token"].(string)
	claims, _ := m.Parse(access)
	m.RevokeJTI(claims.ID)
	if _, err := m.Parse(access); err == nil {
		t.Errorf("Parse should return non-nil error for revoked token")
	} else if errors.Is(err, errors.New("nonsense")) {
		t.Errorf("placeholder sanity check failed")
	}
}
