package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
)

// httptestNewReq is a thin wrapper that lets bulk tests build unauthenticated
// requests without dragging in the net/http/httptest import everywhere.
func httptestNewReq(method, path, body string) *http.Request {
	if body == "" {
		return httptest.NewRequest(method, path, nil)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}
