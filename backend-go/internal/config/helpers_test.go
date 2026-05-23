package config

import (
	"os"
	"path/filepath"
	"testing"
)

// envOrFile reads $NAME, falling back to the contents of $NAME_FILE, then
// the supplied default. We have to test each branch.

func TestEnvOrFile_EnvPresent(t *testing.T) {
	t.Setenv("TEST_VAR_A", "from-env")
	if got := envOrFile("TEST_VAR_A", "default"); got != "from-env" {
		t.Errorf("env wins, got %q", got)
	}
}

func TestEnvOrFile_FileFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("  from-file  \n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("TEST_VAR_B", "")
	t.Setenv("TEST_VAR_B_FILE", path)
	if got := envOrFile("TEST_VAR_B", "default"); got != "from-file" {
		t.Errorf("file should be read and trimmed, got %q", got)
	}
}

func TestEnvOrFile_FileMissingFallsBackToDefault(t *testing.T) {
	t.Setenv("TEST_VAR_C", "")
	t.Setenv("TEST_VAR_C_FILE", "/does/not/exist")
	if got := envOrFile("TEST_VAR_C", "default-val"); got != "default-val" {
		t.Errorf("expected default when file missing, got %q", got)
	}
}

func TestEnvOrFile_DefaultWhenAllUnset(t *testing.T) {
	t.Setenv("TEST_VAR_D", "")
	t.Setenv("TEST_VAR_D_FILE", "")
	if got := envOrFile("TEST_VAR_D", "fallback"); got != "fallback" {
		t.Errorf("expected fallback")
	}
}

// envInt parses to int; bad input returns default.

func TestEnvInt_Parses(t *testing.T) {
	t.Setenv("TEST_INT_A", "42")
	if got := envInt("TEST_INT_A", 100); got != 42 {
		t.Errorf("got %d", got)
	}
}

func TestEnvInt_DefaultsOnUnset(t *testing.T) {
	t.Setenv("TEST_INT_B", "")
	if got := envInt("TEST_INT_B", 7); got != 7 {
		t.Errorf("expected default 7, got %d", got)
	}
}

func TestEnvInt_DefaultsOnBadInput(t *testing.T) {
	t.Setenv("TEST_INT_C", "not-a-number")
	if got := envInt("TEST_INT_C", 99); got != 99 {
		t.Errorf("expected default for bad input, got %d", got)
	}
}

// splitCSV trims and drops empty entries.

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{}},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"  a , b , c  ", []string{"a", "b", "c"}},
		{"a,,b", []string{"a", "b"}},
		{",,,", []string{}},
	}
	for _, c := range cases {
		got := splitCSV(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitCSV(%q): len %d, want %d (%v)", c.in, len(got), len(c.want), got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitCSV(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

// Load() — exercise success + failure modes via env vars. Each call is
// hermetic via t.Setenv.

func TestLoad_FailsWithoutJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("MASTER_ENCRYPTION_KEY", "")
	if _, err := Load(); err == nil {
		t.Errorf("expected error when JWT_SECRET is empty")
	}
}

func TestLoad_FailsWithShortJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "short")
	t.Setenv("MASTER_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if _, err := Load(); err == nil {
		t.Errorf("expected error for short JWT secret")
	}
}

func TestLoad_FailsWithoutMasterKey(t *testing.T) {
	t.Setenv("JWT_SECRET", "this-is-a-long-enough-jwt-secret-32+")
	t.Setenv("MASTER_ENCRYPTION_KEY", "")
	if _, err := Load(); err == nil {
		t.Errorf("expected error for missing master key")
	}
}

func TestLoad_FailsOnBadMasterKey(t *testing.T) {
	t.Setenv("JWT_SECRET", "this-is-a-long-enough-jwt-secret-32+")
	t.Setenv("MASTER_ENCRYPTION_KEY", "not-hex")
	if _, err := Load(); err == nil {
		t.Errorf("expected error for non-hex master key")
	}
}

func TestLoad_FailsOnShortMasterKey(t *testing.T) {
	t.Setenv("JWT_SECRET", "this-is-a-long-enough-jwt-secret-32+")
	// 16 bytes of hex = 8 bytes decoded — too short for AES-256.
	t.Setenv("MASTER_ENCRYPTION_KEY", "0123456789abcdef")
	if _, err := Load(); err == nil {
		t.Errorf("expected error for too-short master key")
	}
}

func TestLoad_Success(t *testing.T) {
	t.Setenv("JWT_SECRET", "this-is-a-long-enough-jwt-secret-32+")
	t.Setenv("MASTER_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("DB_HOST", "test-db")
	t.Setenv("DB_PORT", "3307")
	t.Setenv("ALLOWED_ORIGINS", "https://a.example.com,https://b.example.com")
	t.Setenv("ACCESS_TOKEN_TTL_SECONDS", "300")
	t.Setenv("REFRESH_TOKEN_TTL_SECONDS", "3600")
	t.Setenv("LOG_FORMAT", "text")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AccessTokenTTL != 300 {
		t.Errorf("AccessTokenTTL = %d", cfg.AccessTokenTTL)
	}
	if cfg.RefreshTokenTTL != 3600 {
		t.Errorf("RefreshTokenTTL = %d", cfg.RefreshTokenTTL)
	}
	if len(cfg.AllowedOrigins) != 2 {
		t.Errorf("expected 2 origins, got %v", cfg.AllowedOrigins)
	}
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat = %q", cfg.LogFormat)
	}
}

func TestLoad_FailsValidationOnZeroAccessTTL(t *testing.T) {
	t.Setenv("JWT_SECRET", "this-is-a-long-enough-jwt-secret-32+")
	t.Setenv("MASTER_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("ACCESS_TOKEN_TTL_SECONDS", "0")
	if _, err := Load(); err == nil {
		t.Errorf("expected validation error for zero access TTL")
	}
}
