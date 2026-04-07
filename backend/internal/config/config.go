package config

import (
	"bufio"
	"os"
	"strings"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string

	JWTSecret          string
	MasterEncryptionKey string

	StoragePath    string
	AllowedOrigins string
	RedisHost      string
	RedisPort      string

	MyclouVersion      string
	MycloudGithubRepo  string
	UpdateCommand      string
	UpdateLogPath      string
	UpdaterURL         string
	MycloudProjectDir  string
	UpdateServices     string
}

func Load() *Config {
	return &Config{
		DBHost:              envOrFile("DB_HOST", "mariadb"),
		DBPort:              envOrFile("DB_PORT", "3306"),
		DBName:              envOrFile("DB_NAME", "mycloud"),
		DBUser:              envOrFile("DB_USER", "mycloud"),
		DBPassword:          envOrFile("DB_PASSWORD", ""),
		JWTSecret:           envOrFile("JWT_SECRET", ""),
		MasterEncryptionKey: envOrFile("MASTER_ENCRYPTION_KEY", ""),
		StoragePath:         envOrFile("STORAGE_PATH", "/data/files"),
		AllowedOrigins:      envOrFile("ALLOWED_ORIGINS", "https://localhost"),
		RedisHost:           envOrFile("REDIS_HOST", "redis"),
		RedisPort:           envOrFile("REDIS_PORT", "6379"),
		MyclouVersion:       envOrDefault("MYCLOUD_VERSION", "v0.0.0-dev"),
		MycloudGithubRepo:   envOrDefault("MYCLOUD_GITHUB_REPO", "iii123iii/MyCloud"),
		UpdateCommand:       envOrDefault("MYCLOUD_UPDATE_COMMAND", ""),
		UpdateLogPath:       envOrDefault("MYCLOUD_UPDATE_LOG_PATH", "/data/logs/update.log"),
		UpdaterURL:          envOrDefault("MYCLOUD_UPDATER_URL", ""),
		MycloudProjectDir:   envOrDefault("MYCLOUD_PROJECT_DIR", "/opt/mycloud"),
		UpdateServices:      envOrDefault("MYCLOUD_UPDATE_SERVICES", "backend frontend nginx"),
	}
}

// envOrFile reads an env var directly, or falls back to reading from <name>_FILE path.
func envOrFile(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	if fp := os.Getenv(name + "_FILE"); fp != "" {
		if f, err := os.Open(fp); err == nil {
			defer f.Close()
			scanner := bufio.NewScanner(f)
			if scanner.Scan() {
				if line := strings.TrimSpace(scanner.Text()); line != "" {
					return line
				}
			}
		}
	}
	return def
}

func envOrDefault(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}
