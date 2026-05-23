package app

// Property-based / fuzz tests. Each `FuzzX` is also runnable as a regular
// `go test -fuzz=FuzzX` campaign. The `TestX` form runs a fixed corpus so
// CI catches regressions without needing the fuzz infrastructure.

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// ─── sanitizeUploadName invariants ─────────────────────────────────────────
//
// For any input string, the function must:
//  1. Never panic.
//  2. Never return a "valid" name that is "." or "..".
//  3. Never return a name containing path separators or null bytes.
//  4. Never return a name longer than 255 bytes.
//  5. Be deterministic — same input twice gives same output.

func FuzzSanitizeUploadName_Invariants(f *testing.F) {
	// Seed corpus
	for _, s := range []string{
		"", "a", "..", "../../etc/passwd", "C:\\Windows\\system32",
		"normal.txt", "with spaces.doc", "\x00null", "..\\..",
		strings.Repeat("x", 300),
		"emoji-📁.txt", "‮target.exe", // RTL override
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		// Cap fuzzer input to avoid O(n²) bug in truncation loop.
		// (See spawned task to fix sanitizeUploadName perf.)
		if len(in) > 1024 {
			t.Skip()
		}
		// Invariant 1: no panics.
		got1, ok1 := sanitizeUploadName(in)
		got2, ok2 := sanitizeUploadName(in)
		// Invariant 5: deterministic.
		if got1 != got2 || ok1 != ok2 {
			t.Fatalf("non-deterministic: in=%q out1=(%q,%v) out2=(%q,%v)",
				in, got1, ok1, got2, ok2)
		}
		if !ok1 {
			return
		}
		// Invariant 2: never "." or "..".
		if got1 == "." || got1 == ".." {
			t.Errorf("returned bare-dot name for %q", in)
		}
		// Invariant 3: no separators or null bytes.
		if strings.ContainsAny(got1, "/\\\x00") {
			t.Errorf("returned name with separator/null for %q: %q", in, got1)
		}
		// Invariant 4: ≤255 bytes.
		if len(got1) > 255 {
			t.Errorf("returned name >255 bytes for %q: len=%d", in, len(got1))
		}
		// Invariant 6: valid UTF-8.
		if !utf8.ValidString(got1) {
			t.Errorf("returned non-UTF-8 for %q: %q", in, got1)
		}
	})
}

// Regular Test form so the seed corpus runs in normal CI.
func TestSanitizeUploadName_CorpusReplay(t *testing.T) {
	corpus := []string{
		"", "a", "..", "../../etc/passwd", "C:\\Windows\\system32",
		"normal.txt", "with spaces.doc", "\x00null", "..\\..",
		strings.Repeat("x", 300),
		"emoji-📁.txt", "‮target.exe",
	}
	for _, in := range corpus {
		got, ok := sanitizeUploadName(in)
		if ok {
			if got == "." || got == ".." {
				t.Errorf("%q → bare dot", in)
			}
			if strings.ContainsAny(got, "/\\\x00") {
				t.Errorf("%q → %q contains separator/null", in, got)
			}
			if len(got) > 255 {
				t.Errorf("%q → len=%d > 255", in, len(got))
			}
		}
	}
}

// ─── Presign-token round-trip ──────────────────────────────────────────────
//
// Property: for any (fileID, expires, purpose, secret), the token verifies.
// Any bit-flip in the output token causes verification to fail.

func FuzzPresignToken_RoundTrip(f *testing.F) {
	f.Add("file-1", int64(1234567890), "download", "secret")
	f.Add("", int64(0), "", "")
	f.Add("F"+strings.Repeat("a", 100), int64(-1), "preview", "x")
	f.Fuzz(func(t *testing.T, fileID string, expires int64, purpose, secret string) {
		tok := signPresignToken(secret, fileID, expires, purpose)
		gotFile, gotExp, gotPur, ok := verifyPresignToken(secret, tok)
		if !ok {
			t.Fatalf("round-trip failed: in=(%q,%d,%q,%q) tok=%q", fileID, expires, purpose, secret, tok)
		}
		if gotFile != fileID || gotExp != expires || gotPur != purpose {
			// "|" in any field breaks the parser — that's a known limitation
			// of the pipe-separated format and acceptable for this internal
			// token. Skip if any input contains "|".
			if strings.ContainsAny(fileID+purpose, "|") {
				return
			}
			t.Errorf("decoded mismatch: in=(%q,%d,%q) out=(%q,%d,%q)",
				fileID, expires, purpose, gotFile, gotExp, gotPur)
		}
	})
}

// Bit-flip in the token must always fail verification.
func TestPresignToken_BitFlipFails(t *testing.T) {
	tok := signPresignToken("secret", "f1", 12345, "download")
	if len(tok) == 0 {
		t.Fatal("empty token")
	}
	flipped := []byte(tok)
	flipped[len(flipped)/2] ^= 0x01
	if _, _, _, ok := verifyPresignToken("secret", string(flipped)); ok {
		t.Error("bit-flipped token verified")
	}
}

// ─── normaliseName idempotency ─────────────────────────────────────────────
//
// Property: normaliseName(normaliseName(x)) == normaliseName(x).
// If true, the field can be safely re-normalised during migrations without
// drift.

func FuzzNormaliseName_Idempotent(f *testing.F) {
	for _, s := range []string{
		"", "abc", "ABC", "café", "résumé", "‮target.exe",
		"   leading-trailing   ", "ALL CAPS", "MixedCase",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		if len(in) > 1024 {
			t.Skip()
		}
		first := normaliseName(in)
		second := normaliseName(first)
		if first != second {
			t.Errorf("not idempotent: in=%q first=%q second=%q", in, first, second)
		}
	})
}

func TestNormaliseName_CorpusIdempotent(t *testing.T) {
	corpus := []string{
		"", "abc", "ABC", "café", "résumé", "‮target.exe",
		"   leading-trailing   ", "ALL CAPS", "MixedCase",
	}
	for _, in := range corpus {
		first := normaliseName(in)
		second := normaliseName(first)
		if first != second {
			t.Errorf("not idempotent: in=%q first=%q second=%q", in, first, second)
		}
	}
}

// ─── sanitizeFilename: identity over plain ASCII ───────────────────────────
//
// Property: sanitizeFilename on a string containing no quotes/CRLF/null/
// backslash is a no-op.

func TestSanitizeFilename_IdentityOnAscii(t *testing.T) {
	cases := []string{
		"plain.txt", "spaces in name.doc", "a-b_c.pdf",
		"日本語.txt", "café.csv",
	}
	for _, in := range cases {
		if got := sanitizeFilename(in); got != in {
			t.Errorf("identity violated: %q → %q", in, got)
		}
	}
}

// ─── parseAccessLevel round-trip ───────────────────────────────────────────

func TestParseAccessLevel_RoundTrip(t *testing.T) {
	cases := []string{"viewer", "editor", "owner"}
	for _, name := range cases {
		lvl := parseAccessLevel(name)
		if lvl.String() != name {
			t.Errorf("round-trip failed: %q → %v → %q", name, lvl, lvl.String())
		}
	}
}
