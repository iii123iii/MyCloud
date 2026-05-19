package db

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"

	"mycloud/backend-go/migrations"
)

// Migrate brings the schema up to date by running every *.sql file in the
// embedded migrations FS that has not yet been recorded in schema_migrations.
//
// Existing installs (created before this runner existed) are detected by the
// presence of the `users` table together with an empty schema_migrations.
// In that case we pre-seed versions 001 and 002 so they are not re-applied.
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    VARCHAR(64) NOT NULL,
		applied_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (version)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	var hasUsersTable int
	_ = db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'users'`).Scan(&hasUsersTable)
	var appliedCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&appliedCount); err != nil {
		return fmt.Errorf("count schema_migrations: %w", err)
	}
	if hasUsersTable > 0 && appliedCount == 0 {
		for _, v := range []string{"001", "002"} {
			if _, err := db.Exec("INSERT IGNORE INTO schema_migrations (version) VALUES (?)", v); err != nil {
				return fmt.Errorf("seed legacy migration %s: %w", v, err)
			}
		}
	}

	applied, err := loadApplied(db)
	if err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		version := strings.SplitN(name, "_", 2)[0]
		if applied[version] {
			continue
		}
		body, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		log.Printf("migrations: applying %s", name)
		if err := execStatements(db, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			return fmt.Errorf("record %s: %w", name, err)
		}
	}
	return nil
}

func loadApplied(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("load schema_migrations: %w", err)
	}
	defer rows.Close()
	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// execStatements runs each ;-delimited statement in sqlText against db.
// The MySQL driver does not allow multi-statement queries by default; this
// splitter respects single/double/backtick string literals and skips -- line
// comments so they do not confuse the boundary detection.
func execStatements(db *sql.DB, sqlText string) error {
	for _, stmt := range splitSQL(sqlText) {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			continue
		}
		if _, err := db.Exec(trimmed); err != nil {
			return fmt.Errorf("stmt failed: %s\n%w", firstLine(trimmed), err)
		}
	}
	return nil
}

func splitSQL(s string) []string {
	var out []string
	var b strings.Builder
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			b.WriteByte(c)
			if c == '\\' && i+1 < len(s) {
				b.WriteByte(s[i+1])
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			quote = c
			b.WriteByte(c)
			continue
		}
		if c == '-' && i+1 < len(s) && s[i+1] == '-' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			b.WriteByte('\n')
			continue
		}
		if c == ';' {
			out = append(out, b.String())
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	if strings.TrimSpace(b.String()) != "" {
		out = append(out, b.String())
	}
	return out
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
