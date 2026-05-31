//go:build integration

package db

// Real-database migration tests. These run the embedded migrations against a
// live MariaDB 11 — matching production — to catch what the sqlmock and
// static-integrity unit tests cannot: SQL syntax errors, splitter
// incompatibilities, ordering violations and (most importantly) whether a
// fresh install AND an upgrade from every prior version both reach head.
//
// Run with:
//
//	MYCLOUD_TEST_DSN='root:testroot@tcp(127.0.0.1:3307)/' \
//	    go test -tags integration -run TestMigrateIntegration ./internal/db/ -v
//
// The DSN must point at a throwaway server with privileges to DROP/CREATE
// DATABASE: every test recreates a scratch database. Without the env var the
// tests skip, so plain `go test ./...` is unaffected.

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"mycloud/backend-go/migrations"
)

const testDBName = "mycloud_migtest"

// baseDSN returns the root DSN (no database selected), e.g.
// "root:testroot@tcp(127.0.0.1:3307)/", or skips if MYCLOUD_TEST_DSN is unset.
func baseDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("MYCLOUD_TEST_DSN")
	if dsn == "" {
		t.Skip("MYCLOUD_TEST_DSN not set; skipping migration integration tests")
	}
	if !strings.HasSuffix(dsn, "/") {
		t.Fatalf("MYCLOUD_TEST_DSN must end in / with no database selected, got %q", dsn)
	}
	return dsn
}

// dbDSN points at the scratch database using the same driver params the
// production runner connects with (internal/config/config.go).
func dbDSN(base string) string {
	return base + testDBName + "?parseTime=true&charset=utf8mb4,utf8&clientFoundRows=true"
}

// freshDB drops and recreates the scratch database and returns a pool to it.
func freshDB(t *testing.T, base string) *sql.DB {
	t.Helper()
	root, err := sql.Open("mysql", base)
	if err != nil {
		t.Fatalf("open root dsn: %v", err)
	}
	defer root.Close()
	if _, err := root.Exec("DROP DATABASE IF EXISTS " + testDBName); err != nil {
		t.Fatalf("drop database: %v", err)
	}
	if _, err := root.Exec("CREATE DATABASE " + testDBName +
		" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("create database: %v", err)
	}
	db, err := sql.Open("mysql", dbDSN(base))
	if err != nil {
		t.Fatalf("open scratch dsn: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping scratch db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// sortedMigrationFiles lists the embedded *.sql files in the same apply order
// the runner uses.
func sortedMigrationFiles(t *testing.T) []string {
	t.Helper()
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no migration files found")
	}
	return files
}

func versionOf(name string) string { return strings.SplitN(name, "_", 2)[0] }

// applyThrough reproduces an install that stopped at files[k-1]: it applies
// migration bodies 001..k via the same splitter the runner uses and records
// each version, exactly as a real deployment at that version would have.
func applyThrough(t *testing.T, db *sql.DB, files []string, k int) {
	t.Helper()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    VARCHAR(64) NOT NULL,
		applied_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (version)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	for i := 0; i < k; i++ {
		body, err := fs.ReadFile(migrations.FS, files[i])
		if err != nil {
			t.Fatalf("read %s: %v", files[i], err)
		}
		if err := execStatements(db, string(body)); err != nil {
			t.Fatalf("seed-apply %s: %v", files[i], err)
		}
		if _, err := db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", versionOf(files[i])); err != nil {
			t.Fatalf("seed-record %s: %v", files[i], err)
		}
	}
}

func appliedVersions(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	m, err := loadApplied(db)
	if err != nil {
		t.Fatalf("loadApplied: %v", err)
	}
	return m
}

func count(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	return count(t, db, `SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = ?`, name) > 0
}

func columnExists(t *testing.T, db *sql.DB, table, col string) bool {
	return count(t, db, `SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`, table, col) > 0
}

func indexExists(t *testing.T, db *sql.DB, table, index string) bool {
	return count(t, db, `SELECT COUNT(*) FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`, table, index) > 0
}

// assertReachedHead verifies every migration version is recorded and
// spot-checks artefacts from across history. The spot-checks matter because a
// guarded statement that silently skips real work (e.g. a connection-scoped
// guard that lost its session variable) returns no error but leaves the
// schema wrong — only an existence check catches that.
func assertReachedHead(t *testing.T, db *sql.DB) {
	t.Helper()
	files := sortedMigrationFiles(t)
	applied := appliedVersions(t, db)
	for _, f := range files {
		if !applied[versionOf(f)] {
			t.Errorf("version %s not recorded", versionOf(f))
		}
	}
	for _, tbl := range []string{
		"users", "files", "folders", "shares", "share_grants", "activity_log",
		"blob_refs", "file_versions", "comments", "tags", "upload_requests",
		"file_locks", "retention_policies", "saved_queries", "slow_queries",
		"hls_jobs", "presign_audit", "sessions", "invite_tokens", "user_favorites",
		"personal_access_tokens", "webhooks", "webhook_deliveries",
	} {
		if !tableExists(t, db, tbl) {
			t.Errorf("table %q missing at head", tbl)
		}
	}
	// 015 — activity_log diff columns.
	for _, c := range []string{"old_value", "new_value", "operation_id", "reversible", "rolled_back_at"} {
		if !columnExists(t, db, "activity_log", c) {
			t.Errorf("activity_log.%s (015) missing", c)
		}
	}
	// 020 — normalised name column + fulltext index.
	if !columnExists(t, db, "files", "name_normalised") {
		t.Errorf("files.name_normalised (020) missing")
	}
	if !indexExists(t, db, "files", "ft_files_name_normalised") {
		t.Errorf("fulltext ft_files_name_normalised (020) missing")
	}
	// 025 — one-time-view share columns.
	for _, c := range []string{"single_view", "viewed_at"} {
		if !columnExists(t, db, "shares", c) {
			t.Errorf("shares.%s (025) missing", c)
		}
	}
	// 027 — the prepared-statement-guarded indexes MUST actually be created.
	for _, idx := range []string{"idx_share_grants_grantee_file", "idx_share_grants_grantee_folder"} {
		if !indexExists(t, db, "share_grants", idx) {
			t.Errorf("share_grants.%s (027) missing — guard skipped real work", idx)
		}
	}
}

// A clean database goes from nothing to head.
func TestMigrateIntegration_FreshInstall(t *testing.T) {
	db := freshDB(t, baseDSN(t))
	if err := Migrate(db); err != nil {
		t.Fatalf("fresh Migrate: %v", err)
	}
	assertReachedHead(t, db)
}

// Running the runner twice is a no-op the second time and never errors.
func TestMigrateIntegration_RunnerIdempotent(t *testing.T) {
	db := freshDB(t, baseDSN(t))
	if err := Migrate(db); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	before := len(appliedVersions(t, db))
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if after := len(appliedVersions(t, db)); before != after {
		t.Errorf("applied count changed on re-run: %d -> %d", before, after)
	}
	assertReachedHead(t, db)
}

// Re-executing each migration body against the already-migrated schema must
// succeed. The runner re-runs a whole file from the top when a prior apply
// died partway, so every statement has to tolerate already-applied state.
// This is the actual idempotency contract the IF NOT EXISTS guards exist for.
func TestMigrateIntegration_EachFileReapplyable(t *testing.T) {
	db := freshDB(t, baseDSN(t))
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	for _, f := range sortedMigrationFiles(t) {
		body, err := fs.ReadFile(migrations.FS, f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if err := execStatements(db, string(body)); err != nil {
			t.Errorf("re-applying %s is not idempotent: %v", f, err)
		}
	}
	assertReachedHead(t, db)
}

// The core "works for every version" matrix: for each historical version k,
// stand up an install frozen at k and confirm the current runner carries it
// all the way to head.
func TestMigrateIntegration_UpgradeFromEveryVersion(t *testing.T) {
	base := baseDSN(t)
	files := sortedMigrationFiles(t)
	for k := 1; k < len(files); k++ {
		from := versionOf(files[k-1])
		t.Run(fmt.Sprintf("from_%s", from), func(t *testing.T) {
			db := freshDB(t, base)
			applyThrough(t, db, files, k)
			if err := Migrate(db); err != nil {
				t.Fatalf("upgrade from %s: %v", from, err)
			}
			assertReachedHead(t, db)
		})
	}
}

// Reproduces a real fresh install: the mariadb container auto-runs 001+002
// from docker-entrypoint-initdb.d before the Go runner sees the database,
// leaving schema_migrations empty. Migrate must seed 001/002 as already-applied
// and continue with 003+ rather than trying to recreate existing tables.
func TestMigrateIntegration_LegacyInitdbSeed(t *testing.T) {
	db := freshDB(t, baseDSN(t))
	files := sortedMigrationFiles(t)
	for _, f := range files[:2] { // initdb.d mounts exactly 001 and 002
		body, err := fs.ReadFile(migrations.FS, f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if err := execStatements(db, string(body)); err != nil {
			t.Fatalf("initdb-apply %s: %v", f, err)
		}
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate after initdb seed: %v", err)
	}
	if applied := appliedVersions(t, db); !applied["001"] || !applied["002"] {
		t.Errorf("legacy 001/002 not seeded as applied: %v", applied)
	}
	assertReachedHead(t, db)
}
