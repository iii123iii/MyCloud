package dsl

import "testing"

// compileResult wraps Compile's return for ergonomic per-test inspection.
type compileResult struct {
	Where string
	Args  []any
}

// sqlGlob is unexported but exercised through the Compile path that emits
// LIKE clauses. Test the translation table directly via a package-level
// thin wrapper. We can't call sqlGlob from outside the package; instead,
// hit Compile with a wildcard pattern and inspect the generated SQL+args.
func TestCompile_WildcardTranslatedToLIKE(t *testing.T) {
	// Wildcard query with * should produce a LIKE clause.
	q, err := Parse(`name:foo*bar`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	where, args := Compile(q, "u-test")
	got := compileResult{Where: where, Args: args}
	if got.Where == "" {
		t.Errorf("expected non-empty WHERE for wildcard query")
	}
	// Inspect args: should include a value with % (the * translation).
	foundPercent := false
	for _, a := range got.Args {
		if s, ok := a.(string); ok {
			for _, ch := range s {
				if ch == '%' {
					foundPercent = true
				}
			}
		}
	}
	if !foundPercent {
		t.Errorf("expected at least one arg to contain %% from * translation, got %v", got.Args)
	}
}

// Question-mark translates to underscore.
func TestCompile_SingleCharWildcardToUnderscore(t *testing.T) {
	q, err := Parse(`name:a?b`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	where, args := Compile(q, "u-test")
	got := compileResult{Where: where, Args: args}
	foundUnderscore := false
	for _, a := range got.Args {
		if s, ok := a.(string); ok {
			for _, ch := range s {
				if ch == '_' {
					foundUnderscore = true
				}
			}
		}
	}
	if !foundUnderscore {
		t.Errorf("expected _ from ? translation, got %v", got.Args)
	}
}

// Mixed wildcard + literal: foo%* should escape the literal % and
// translate the *. Probes the sqlGlob escape branch.
func TestCompile_MixedWildcardAndLiteral(t *testing.T) {
	q, err := Parse(`name:foo%*`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	where, args := Compile(q, "u-test")
	got := compileResult{Where: where, Args: args}
	if got.Where == "" {
		t.Errorf("expected non-empty WHERE")
	}
	// The argument should contain both \% (escaped literal) and % (the
	// translated *).
	hasEscape := false
	hasWildcard := false
	for _, a := range got.Args {
		if s, ok := a.(string); ok {
			for i := 0; i < len(s); i++ {
				if i+1 < len(s) && s[i] == '\\' && s[i+1] == '%' {
					hasEscape = true
				}
			}
			// Wildcard % appears at the end (from *).
			if len(s) > 0 && s[len(s)-1] == '%' {
				hasWildcard = true
			}
		}
	}
	if !hasEscape {
		t.Errorf("expected escape of literal %%, got %v", got.Args)
	}
	if !hasWildcard {
		t.Errorf("expected translated wildcard at end, got %v", got.Args)
	}
}
