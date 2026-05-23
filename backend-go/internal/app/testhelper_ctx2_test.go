package app

import "context"

// testCtx returns a fresh background context for unit tests that need to
// pass one through to helpers (e.g. collectFolderTree). Keeps every call
// site to a one-liner.
func testCtx() context.Context { return context.Background() }
