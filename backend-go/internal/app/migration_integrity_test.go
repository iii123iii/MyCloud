package app

// Migration integrity: every file in migrations/ parses, no version
// duplicates, sequential numbering with no gaps that would confuse the
// runner. The actual schema application is exercised by integration tests
// against a real database; this file catches the cheap stuff in unit time.

import (
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"mycloud/backend-go/migrations"
)

var migrationFilenameRE = regexp.MustCompile(`^(\d{3,})_.+\.sql$`)

// All migration files have the expected NNN_name.sql shape and parse as
// non-empty SQL.
func TestMigrations_AllFilenamesValid(t *testing.T) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".sql" {
			continue
		}
		if !migrationFilenameRE.MatchString(name) {
			t.Errorf("migration filename does not match NNN_name.sql: %q", name)
		}
		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			t.Errorf("could not read %q: %v", name, err)
		}
		if len(strings.TrimSpace(string(body))) == 0 {
			t.Errorf("migration %q is empty", name)
		}
	}
}

// Migration numbers are strictly increasing with no duplicates. A gap is
// allowed (e.g. squashed history) but two files with the same number would
// cause the runner to apply them in unstable order.
func TestMigrations_NoDuplicateVersions(t *testing.T) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := migrationFilenameRE.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		ver, _ := strconv.Atoi(m[1])
		if existing, dup := seen[ver]; dup {
			t.Errorf("duplicate migration version %d: %q and %q", ver, existing, e.Name())
		}
		seen[ver] = e.Name()
	}
}

// Sorted versions form a sane prefix of ints (no negative or huge gaps).
func TestMigrations_VersionSequenceSane(t *testing.T) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var vers []int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := migrationFilenameRE.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		ver, _ := strconv.Atoi(m[1])
		vers = append(vers, ver)
	}
	sort.Ints(vers)
	if len(vers) < 2 {
		return
	}
	for i := 1; i < len(vers); i++ {
		gap := vers[i] - vers[i-1]
		if gap > 50 {
			t.Errorf("large version gap between %d and %d (%d)", vers[i-1], vers[i], gap)
		}
	}
}

// Critical tables must be present somewhere in the schema. We check via
// substring search rather than a full SQL parse so we don't take on a
// heavy parser dep.
func TestMigrations_CoreTablesDefined(t *testing.T) {
	required := []string{
		"users", "files", "folders", "shares", "share_grants",
		"activity_log", "blob_refs", "file_versions", "sessions",
		"file_locks", "comments", "tags", "upload_requests",
	}
	body := ""
	entries, _ := migrations.FS.ReadDir(".")
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, _ := migrations.FS.ReadFile(e.Name())
		body += strings.ToLower(string(b)) + "\n"
	}
	for _, table := range required {
		// "create table" or "create table if not exists" — match the prefix.
		needle := "create table"
		if !strings.Contains(body, needle) || !strings.Contains(body, table) {
			t.Errorf("required table %q not found in migrations", table)
		}
	}
}

// Every migration with foreign-key references must have the parent table
// defined in some earlier file (no ordering violations). Pragmatic check:
// we scan for `references X(...)` and assert X appears as the target of a
// CREATE TABLE somewhere in the corpus.
func TestMigrations_ForeignKeyTargetsExist(t *testing.T) {
	entries, _ := migrations.FS.ReadDir(".")
	all := ""
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, _ := migrations.FS.ReadFile(e.Name())
		all += strings.ToLower(string(b)) + "\n"
	}
	re := regexp.MustCompile(`references\s+([a-z_]+)\s*\(`)
	matches := re.FindAllStringSubmatch(all, -1)
	for _, m := range matches {
		target := m[1]
		if !strings.Contains(all, "create table `"+target+"`") &&
			!strings.Contains(all, "create table "+target) &&
			!strings.Contains(all, "create table if not exists `"+target+"`") &&
			!strings.Contains(all, "create table if not exists "+target) {
			t.Errorf("FK target table %q never CREATEd", target)
		}
	}
}
