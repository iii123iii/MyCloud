package config

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr          string
	DBDSN               string
	// DBReplicaDSN, when non-empty, enables read-replica routing via
	// internal/db/cluster. Falls back to primary when empty.
	DBReplicaDSN string
	AllowedOrigins      []string
	JWTSecret           string
	AccessTokenTTL      int
	RefreshTokenTTL     int
	RedisAddr           string
	RedisPassword       string
	StoragePath         string
	MasterEncryptionKey string
	UpdaterURL          string
	UpdateLogPath       string
	Version             string
	GitHubRepo          string

	// Rate limit capacity per window. Set capacity to 0 to disable.
	RateLimitLoginPerMin      int
	RateLimitLoginPerUserMin  int
	RateLimitRegisterPerHour  int
	RateLimitSharePwdPerMin   int
	RateLimitPublicUploadHour int
	RateLimitTusPerMin        int
	// Feature-scoped limits. All bucket per user-id when set; "" key
	// falls through (e.g. unauthenticated routes still hit IP-based limits
	// elsewhere). Defaults are generous — these exist to keep one user from
	// monopolising the backend, not to fight abuse (which is what the login/
	// register limits do).
	RateLimitDownloadPerMin int
	RateLimitSearchPerMin   int
	RateLimitGenericPerMin  int

	// Maximum number of historical versions retained per file.
	MaxVersionsPerFile int

	// OnlyOffice Document Server JWT secret + public URL.
	OfficeJWTSecret string
	OfficeURL       string
	// Public-facing URL of THIS backend, used by OnlyOffice to fetch and POST
	// to our document and callback endpoints. When empty we default to the
	// request's Host header at runtime.
	PublicBackendURL string

	// LogFormat is "json" (prod) or "text" (dev). Routed to the root slog
	// handler. Defaults to "json" so Docker captures structured records.
	LogFormat string
	// LogLevel is "debug", "info", "warn", "error". Defaults to "info".
	LogLevel string
	// LogFilePath, when non-empty, mirrors all log records to a JSONL file in
	// addition to stdout. The admin log viewer reads this file. Empty disables
	// the file sink. Defaults to /data/logs/app.jsonl alongside the update log.
	LogFilePath string
}

func Load() (Config, error) {
	dbHost := envOrFile("DB_HOST", "mariadb")
	dbPort := envOrFile("DB_PORT", "3306")
	dbName := envOrFile("DB_NAME", "mycloud")
	dbUser := envOrFile("DB_USER", "mycloud")
	dbPass := envOrFile("DB_PASSWORD", "")
	jwtSecret := envOrFile("JWT_SECRET", "")
	if jwtSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}
	if len(jwtSecret) < 32 {
		// HS256 needs ≥256 bits of entropy for full security. We can't
		// measure the entropy of the secret directly, so we proxy with
		// length — a hex-encoded 32-byte random ≥ 64 chars; a passphrase
		// of plain-ASCII ≥ 32 chars is the practical floor.
		return Config{}, fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}

	masterKey := envOrFile("MASTER_ENCRYPTION_KEY", "")
	if masterKey == "" {
		return Config{}, fmt.Errorf("MASTER_ENCRYPTION_KEY is required")
	}
	// Master key is hex-encoded; decode to verify it yields exactly 32 bytes
	// for AES-256. Catching this here surfaces the misconfig at boot rather
	// than at the first upload.
	if decoded, err := hex.DecodeString(masterKey); err != nil {
		return Config{}, fmt.Errorf("MASTER_ENCRYPTION_KEY must be hex-encoded: %w", err)
	} else if len(decoded) != 32 {
		return Config{}, fmt.Errorf("MASTER_ENCRYPTION_KEY must decode to 32 bytes (got %d)", len(decoded))
	}

	cfg := Config{
		ListenAddr:          envOrFile("LISTEN_ADDR", ":8080"),
		// clientFoundRows=true makes RowsAffected() return the number of rows
		// that MATCHED the WHERE clause, not just those whose values actually
		// changed. Without it, an UPDATE like `SET used_bytes = used_bytes + 0`
		// reports 0 rows affected and the quota check falsely rejects the upload.
		DBDSN:               fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4,utf8&clientFoundRows=true", dbUser, dbPass, dbHost, dbPort, dbName),
		DBReplicaDSN:        envOrFile("DB_REPLICA_DSN", ""),
		AllowedOrigins:      splitCSV(envOrFile("ALLOWED_ORIGINS", "https://localhost")),
		JWTSecret:           jwtSecret,
		AccessTokenTTL:      envInt("ACCESS_TOKEN_TTL_SECONDS", 900),
		RefreshTokenTTL:     envInt("REFRESH_TOKEN_TTL_SECONDS", 604800),
		RedisAddr:           fmt.Sprintf("%s:%s", envOrFile("REDIS_HOST", "redis"), envOrFile("REDIS_PORT", "6379")),
		RedisPassword:       envOrFile("REDIS_PASSWORD", ""),
		StoragePath:         envOrFile("STORAGE_PATH", "/data/files"),
		MasterEncryptionKey: masterKey,
		UpdaterURL:          envOrFile("MYCLOUD_UPDATER_URL", ""),
		UpdateLogPath:       envOrFile("MYCLOUD_UPDATE_LOG_PATH", "/data/logs/update.log"),
		Version:             envOrFile("MYCLOUD_VERSION", "dev"),
		GitHubRepo:          envOrFile("MYCLOUD_GITHUB_REPO", "iii123iii/MyCloud"),

		RateLimitLoginPerMin:      envInt("RATE_LIMIT_LOGIN_PER_MIN", 10),
		RateLimitLoginPerUserMin:  envInt("RATE_LIMIT_LOGIN_PER_USER_MIN", 5),
		RateLimitRegisterPerHour:  envInt("RATE_LIMIT_REGISTER_PER_HOUR", 5),
		RateLimitSharePwdPerMin:   envInt("RATE_LIMIT_SHARE_PWD_PER_MIN", 10),
		RateLimitPublicUploadHour: envInt("RATE_LIMIT_PUBLIC_UPLOAD_PER_HOUR", 30),
		RateLimitTusPerMin:        envInt("RATE_LIMIT_TUS_PER_MIN", 100),
		RateLimitDownloadPerMin:   envInt("RATE_LIMIT_DOWNLOAD_PER_MIN", 240),
		RateLimitSearchPerMin:     envInt("RATE_LIMIT_SEARCH_PER_MIN", 120),
		RateLimitGenericPerMin:    envInt("RATE_LIMIT_GENERIC_PER_MIN", 600),

		MaxVersionsPerFile: envInt("MAX_VERSIONS_PER_FILE", 10),

		OfficeJWTSecret:  envOrFile("OFFICE_JWT_SECRET", ""),
		OfficeURL:        envOrFile("OFFICE_URL", ""),
		PublicBackendURL: envOrFile("MYCLOUD_PUBLIC_BACKEND_URL", ""),

		LogFormat:   envOrFile("LOG_FORMAT", "json"),
		LogLevel:    envOrFile("LOG_LEVEL", "info"),
		LogFilePath: envOrFile("LOG_FILE_PATH", "/data/logs/app.jsonl"),
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// validate runs structural checks beyond the per-field requireds above.
// Logged warnings (not errors) for soft issues — a zero rate-limit
// deliberately disables that bucket and is sometimes intentional, but
// operators should see a clear note that they're running unprotected.
func (c *Config) validate() error {
	if len(c.AllowedOrigins) == 0 {
		slog.Warn("ALLOWED_ORIGINS is empty — CORS will reject every origin until configured")
	}
	if c.RateLimitLoginPerMin <= 0 {
		slog.Warn("RATE_LIMIT_LOGIN_PER_MIN is 0; login brute-force protection disabled")
	}
	if c.RateLimitSharePwdPerMin <= 0 {
		slog.Warn("RATE_LIMIT_SHARE_PWD_PER_MIN is 0; share-password brute-force protection disabled")
	}
	if c.AccessTokenTTL <= 0 {
		return fmt.Errorf("ACCESS_TOKEN_TTL_SECONDS must be positive (got %d)", c.AccessTokenTTL)
	}
	if c.RefreshTokenTTL <= 0 {
		return fmt.Errorf("REFRESH_TOKEN_TTL_SECONDS must be positive (got %d)", c.RefreshTokenTTL)
	}
	if c.RefreshTokenTTL <= c.AccessTokenTTL {
		return fmt.Errorf("REFRESH_TOKEN_TTL (%d) must exceed ACCESS_TOKEN_TTL (%d)",
			c.RefreshTokenTTL, c.AccessTokenTTL)
	}
	if c.MaxVersionsPerFile < 1 {
		return fmt.Errorf("MAX_VERSIONS_PER_FILE must be at least 1 (got %d)", c.MaxVersionsPerFile)
	}
	return nil
}

func envOrFile(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	if fp := os.Getenv(name + "_FILE"); fp != "" {
		if b, err := os.ReadFile(fp); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return def
}

func envInt(name string, def int) int {
	v := envOrFile(name, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
