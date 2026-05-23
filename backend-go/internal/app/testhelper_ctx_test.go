package app

import (
	"context"
	"net/http"
)

// contextWithMustChange threads the must_change_password flag into a
// request's context for middleware tests. requireAuth sets this in
// production; tests bypass that and set it directly.
func contextWithMustChange(r *http.Request, v bool) context.Context {
	return context.WithValue(r.Context(), mustChangePwKey, v)
}
