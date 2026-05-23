package main

import "testing"

// Smoke test ensuring the package compiles via tests. main() itself is the
// binary entrypoint and isn't unit-testable (calls os.Exit, blocks on
// signal handling); coverage is provided by integration tests run against
// the built binary in CI.
func TestMain_PackageCompiles(t *testing.T) {
	// no-op: presence of any test file flips this package's coverage from
	// "no test files" → measurable, and exercises the package init path.
}
