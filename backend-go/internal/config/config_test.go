package config

import (
	"strings"
	"testing"
)

// validate is the gatekeeper added in the audit follow-ups. Test that every
// rule fires correctly. We construct a baseline valid Config and mutate one
// field at a time so each failure mode is isolated.

func validBaseline() Config {
	return Config{
		JWTSecret:           strings.Repeat("a", 32),
		MasterEncryptionKey: strings.Repeat("a", 64), // hex = 32 bytes
		AccessTokenTTL:      900,
		RefreshTokenTTL:     604800,
		MaxVersionsPerFile:  10,
		AllowedOrigins:      []string{"https://example.com"},
		RateLimitLoginPerMin:    10,
		RateLimitSharePwdPerMin: 10,
	}
}

func TestValidate_BaselinePasses(t *testing.T) {
	c := validBaseline()
	if err := c.validate(); err != nil {
		t.Fatalf("baseline should validate, got %v", err)
	}
}

func TestValidate_ZeroAccessTTL(t *testing.T) {
	c := validBaseline()
	c.AccessTokenTTL = 0
	if err := c.validate(); err == nil {
		t.Errorf("expected error for zero access TTL")
	}
}

func TestValidate_NegativeAccessTTL(t *testing.T) {
	c := validBaseline()
	c.AccessTokenTTL = -5
	if err := c.validate(); err == nil {
		t.Errorf("expected error for negative access TTL")
	}
}

func TestValidate_ZeroRefreshTTL(t *testing.T) {
	c := validBaseline()
	c.RefreshTokenTTL = 0
	if err := c.validate(); err == nil {
		t.Errorf("expected error for zero refresh TTL")
	}
}

func TestValidate_RefreshShorterThanAccess(t *testing.T) {
	c := validBaseline()
	c.AccessTokenTTL = 1000
	c.RefreshTokenTTL = 500
	if err := c.validate(); err == nil {
		t.Errorf("expected error when refresh <= access TTL")
	}
}

func TestValidate_RefreshEqualToAccess(t *testing.T) {
	c := validBaseline()
	c.AccessTokenTTL = 100
	c.RefreshTokenTTL = 100
	if err := c.validate(); err == nil {
		t.Errorf("equal TTLs should fail (refresh must outlive access)")
	}
}

func TestValidate_ZeroMaxVersions(t *testing.T) {
	c := validBaseline()
	c.MaxVersionsPerFile = 0
	if err := c.validate(); err == nil {
		t.Errorf("expected error for zero max versions")
	}
}

func TestValidate_EmptyOrigins_LogsWarn(t *testing.T) {
	c := validBaseline()
	c.AllowedOrigins = nil
	// Empty origins is a warning, not an error — boot completes, CORS
	// rejects every origin (fail closed). Test that it doesn't return err.
	if err := c.validate(); err != nil {
		t.Errorf("empty origins should warn, not error; got %v", err)
	}
}

func TestValidate_ZeroLoginRateLimit_LogsWarn(t *testing.T) {
	c := validBaseline()
	c.RateLimitLoginPerMin = 0
	if err := c.validate(); err != nil {
		t.Errorf("zero rate limit is a warning, not an error; got %v", err)
	}
}
