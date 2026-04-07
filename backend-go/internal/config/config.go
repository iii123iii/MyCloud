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
		DBDSN:               fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4,utf8", dbUser, dbPass, dbHost, dbPort, dbName),
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
