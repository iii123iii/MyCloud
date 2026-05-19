package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr          string
	DBDSN               string
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

	// Maximum number of historical versions retained per file.
	MaxVersionsPerFile int

	// OnlyOffice Document Server JWT secret + public URL.
	OfficeJWTSecret string
	OfficeURL       string
	// Public-facing URL of THIS backend, used by OnlyOffice to fetch and POST
	// to our document and callback endpoints. When empty we default to the
	// request's Host header at runtime.
	PublicBackendURL string
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

	masterKey := envOrFile("MASTER_ENCRYPTION_KEY", "")
	if masterKey == "" {
		return Config{}, fmt.Errorf("MASTER_ENCRYPTION_KEY is required")
	}

	return Config{
		ListenAddr:          envOrFile("LISTEN_ADDR", ":8080"),
		// clientFoundRows=true makes RowsAffected() return the number of rows
		// that MATCHED the WHERE clause, not just those whose values actually
		// changed. Without it, an UPDATE like `SET used_bytes = used_bytes + 0`
		// reports 0 rows affected and the quota check falsely rejects the upload.
		DBDSN:               fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4,utf8&clientFoundRows=true", dbUser, dbPass, dbHost, dbPort, dbName),
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

		MaxVersionsPerFile: envInt("MAX_VERSIONS_PER_FILE", 10),

		OfficeJWTSecret:  envOrFile("OFFICE_JWT_SECRET", ""),
		OfficeURL:        envOrFile("OFFICE_URL", ""),
		PublicBackendURL: envOrFile("MYCLOUD_PUBLIC_BACKEND_URL", ""),
	}, nil
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
