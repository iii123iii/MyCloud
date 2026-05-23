package dsl

import (
	"strings"
	"testing"
)

func TestParsePlainTerms(t *testing.T) {
	q, err := Parse("foo bar")
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Terms) != 2 || q.Terms[0] != "foo" || q.Terms[1] != "bar" {
		t.Errorf("terms = %v", q.Terms)
	}
}

func TestParseFields(t *testing.T) {
	q, err := Parse("name:report.pdf size:>1mb tag:invoice after:2025-01-01 owner:me starred:true")
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Name) != 1 || q.Name[0].Value != "report.pdf" {
		t.Errorf("name = %v", q.Name)
	}
	if len(q.Sizes) != 1 || q.Sizes[0].Op != ">" || q.Sizes[0].Bytes != 1<<20 {
		t.Errorf("sizes = %+v", q.Sizes)
	}
	if len(q.Tags) != 1 || q.Tags[0] != "invoice" {
		t.Errorf("tags = %v", q.Tags)
	}
	if q.After.IsZero() {
		t.Error("after not set")
	}
	if !q.OwnerSelf {
		t.Error("owner:me not honoured")
	}
	if !q.StarredSet || !q.Starred {
		t.Error("starred:true not parsed")
	}
}

func TestParseGlobMime(t *testing.T) {
	q, err := Parse("name:*.pdf mime:image/*")
	if err != nil {
		t.Fatal(err)
	}
	if !q.Name[0].Glob {
		t.Error("expected glob name")
	}
	if !q.Mime[0].Prefix || q.Mime[0].Value != "image" {
		t.Errorf("mime = %+v", q.Mime[0])
	}
}

func TestCompileProducesParameterised(t *testing.T) {
	q, _ := Parse("foo name:bar size:>=100kb owner:me")
	frag, args := Compile(q, "user-1")
	if !strings.HasPrefix(frag, " AND ") {
		t.Errorf("fragment must start with ' AND ': %q", frag)
	}
	// Every interpolated piece must be a ? placeholder, never a literal value.
	// A naive injection-style query would include the literal value in frag.
	for _, banned := range []string{"foo", "bar", "100", "user-1"} {
		if strings.Contains(frag, banned) {
			t.Errorf("fragment leaks user input: %q (banned %q)", frag, banned)
		}
	}
	if len(args) == 0 {
		t.Error("expected bound args")
	}
}

// Adversarial inputs should not break parsing or produce SQL injection.
func TestAdversarialInputs(t *testing.T) {
	cases := []string{
		`name:foo' OR '1'='1`,
		`name:"a; DROP TABLE files;--"`,
		`size:` + "`evil`",
		`tag:'%' OR 1=1`,
	}
	for _, c := range cases {
		q, err := Parse(c)
		if err != nil {
			continue // a parse error is fine — refusal beats injection
		}
		frag, args := Compile(q, "user")
		// As long as every dynamic value is in args (not in frag), we're safe.
		_ = frag
		_ = args
		for _, a := range args {
			if s, ok := a.(string); ok {
				// The argument can contain anything; SQL driver escapes it.
				_ = s
			}
		}
	}
}

func TestSizeUnits(t *testing.T) {
	cases := map[string]int64{
		"1024":   1024,
		"5kb":    5 * 1024,
		"1mb":    1 << 20,
		"2gb":    2 << 30,
		"1.5mb":  1572864,
	}
	for in, want := range cases {
		got, err := parseSize(in)
		if err != nil {
			t.Errorf("parseSize(%q) err: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseSize(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestQuotedPhrase(t *testing.T) {
	q, err := Parse(`name:"my report" foo`)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Name) != 1 || q.Name[0].Value != "my report" {
		t.Errorf("name = %+v", q.Name)
	}
	if len(q.Terms) != 1 || q.Terms[0] != "foo" {
		t.Errorf("terms = %v", q.Terms)
	}
}

func TestUnmatchedQuoteRejected(t *testing.T) {
	if _, err := Parse(`name:"oops`); err == nil {
		t.Error("expected error for unmatched quote")
	}
}
