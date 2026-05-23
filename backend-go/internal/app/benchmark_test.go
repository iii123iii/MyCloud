package app

// Benchmarks for the hot-path helpers. Run with:
//   go test ./internal/app -bench BenchmarkSanitizeUploadName -benchmem
// Useful for catching performance regressions, e.g. the O(n²) truncation
// loop already fixed in sanitizeUploadName.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func BenchmarkSanitizeUploadName_Short(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = sanitizeUploadName("normal-file.txt")
	}
}

func BenchmarkSanitizeUploadName_AtCap(b *testing.B) {
	in := strings.Repeat("a", 255)
	for i := 0; i < b.N; i++ {
		_, _ = sanitizeUploadName(in)
	}
}

func BenchmarkSanitizeFilename(b *testing.B) {
	in := `"messy\file\name with\rspecial\nchars.txt`
	for i := 0; i < b.N; i++ {
		_ = sanitizeFilename(in)
	}
}

func BenchmarkNormaliseName(b *testing.B) {
	cases := []string{
		"plain.txt",
		"Mixed Case With Spaces.doc",
		"café résumé",
		"日本語ファイル名.txt",
		strings.Repeat("a-b_c.", 30),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = normaliseName(cases[i%len(cases)])
	}
}

func BenchmarkSignPresignToken(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = signPresignToken("secret", "file-uuid", int64(1234567890), "download")
	}
}

func BenchmarkVerifyPresignToken(b *testing.B) {
	tok := signPresignToken("secret", "file-uuid", int64(1234567890), "download")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = verifyPresignToken("secret", tok)
	}
}

func BenchmarkClientIP_Direct(b *testing.B) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.4:1234"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = clientIP(r)
	}
}

func BenchmarkClientIP_XFFChain(b *testing.B) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.4, 198.51.100.1, 10.0.0.1")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = clientIP(r)
	}
}

func BenchmarkPeekJSONField(b *testing.B) {
	body := `{"username":"alice","password":"secret","extra":"field"}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
		_ = peekJSONField(r, "username")
	}
}

func BenchmarkNullableString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = nullableString("not-empty")
		_ = nullableString("")
	}
}

func BenchmarkInClause_Small(b *testing.B) {
	ids := []string{"a", "b", "c", "d", "e"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = inClause(ids)
	}
}

func BenchmarkInClause_Large(b *testing.B) {
	ids := make([]string, 100)
	for i := range ids {
		ids[i] = "uuid-" + string(rune(i))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = inClause(ids)
	}
}
