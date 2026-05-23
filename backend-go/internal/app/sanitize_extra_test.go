package app

import (
	"strings"
	"testing"
)

// More edge cases for sanitizeUploadName beyond what blob_refs_test covers.
// This is the zip-slip + control-char guard at every upload entry point;
// missing a case here is a security hole, so we go broad.
func TestSanitizeUploadName_TraversalVariants(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"../etc/passwd", "passwd", true},
		{"....//passwd", "passwd", true},      // dot-dot-slash bypass attempt
		{"normal/../../etc/passwd", "passwd", true},
		{"/etc/passwd", "passwd", true},        // absolute path
		{"\\\\windows\\\\system32\\\\evil", "evil", true},
		{"a/b/c/../../../../../etc/x", "x", true},
		{"file.txt/", "file.txt", true},        // trailing slash
		{"/", "", false},                       // root only
		{"...", "...", true},                   // three dots: legitimate (not '.' or '..')
	}
	for _, c := range cases {
		got, ok := sanitizeUploadName(c.in)
		if ok != c.ok {
			t.Errorf("sanitizeUploadName(%q): ok=%v want %v", c.in, ok, c.ok)
			continue
		}
		if ok && got != c.want {
			t.Errorf("sanitizeUploadName(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSanitizeUploadName_ControlByteStripping(t *testing.T) {
	// All ASCII control bytes 0x00-0x1F + 0x7F should be removed.
	got, ok := sanitizeUploadName("a\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x7fb.txt")
	if !ok {
		t.Fatalf("expected ok")
	}
	if got != "ab.txt" {
		t.Errorf("control bytes not stripped: %q", got)
	}
}

func TestSanitizeUploadName_UnicodePreserved(t *testing.T) {
	got, ok := sanitizeUploadName("documento ñame 日本語.pdf")
	if !ok {
		t.Fatalf("unicode name rejected")
	}
	if !strings.Contains(got, "ñ") || !strings.Contains(got, "日") {
		t.Errorf("unicode lost: %q", got)
	}
}

func TestSanitizeUploadName_LongName(t *testing.T) {
	long := strings.Repeat("a", 500) + ".txt"
	got, ok := sanitizeUploadName(long)
	if !ok {
		t.Fatalf("long name rejected")
	}
	if len(got) > 255 {
		t.Errorf("expected name capped at 255 bytes, got %d", len(got))
	}
}

func TestSanitizeUploadName_OnlyTraversal(t *testing.T) {
	// All variations of pure-traversal-only inputs must be rejected.
	for _, in := range []string{"../", "../../", "/../", "./.", "..//.."} {
		if got, ok := sanitizeUploadName(in); ok {
			t.Errorf("expected rejection for %q, got %q", in, got)
		}
	}
}

func TestSanitizeUploadName_DotFiles(t *testing.T) {
	// Hidden files (.gitignore, .env) are legitimate names.
	got, ok := sanitizeUploadName(".gitignore")
	if !ok {
		t.Fatalf(".gitignore should be accepted")
	}
	if got != ".gitignore" {
		t.Errorf("got %q, want .gitignore", got)
	}
}

func TestSanitizeUploadName_QuotesAllowed(t *testing.T) {
	// We strip path components and control bytes, but content-disposition-
	// breaking chars like '"' and '\' are valid filename chars on Linux.
	// (We only strip CR/LF, nulls, and slashes.)
	got, ok := sanitizeUploadName(`my "quoted" file.txt`)
	if !ok {
		t.Fatalf("quoted name rejected")
	}
	if !strings.Contains(got, "quoted") {
		t.Errorf("quote handling odd: %q", got)
	}
}

func TestSanitizeUploadName_ExtensionPreserved(t *testing.T) {
	// We must never strip the extension — file kind dispatch depends on it.
	cases := []struct{ in, want string }{
		{"photo.jpg", "photo.jpg"},
		{"archive.tar.gz", "archive.tar.gz"},
		{"name.with.lots.of.dots.txt", "name.with.lots.of.dots.txt"},
		{"../passwords.zip", "passwords.zip"},
	}
	for _, c := range cases {
		got, _ := sanitizeUploadName(c.in)
		if got != c.want {
			t.Errorf("sanitizeUploadName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
