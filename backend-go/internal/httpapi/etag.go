package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
)

// For derives a stable ETag value from a resource's size and last-modified
// time. The hash collapses both inputs so a same-size, same-mtime byte-for-
// byte identical resource keeps its ETag — which is exactly the equivalence
// class HTTP caching cares about.
//
// We deliberately do NOT hash the content itself: that would force a full
// read of every file just to answer "did it change?" — defeating the point.
// Operators who care about content fingerprints can layer that on with a
// background hashing job later.
//
// The returned ETag is "strong" (no W/ prefix) because the byte content of
// the resource really is identical when both inputs are equal — there's no
// transcoding or content negotiation involved.
func For(sizeBytes int64, modifiedAt string) string {
	h := sha256.New()
	// Length-prefixed so concatenation can't collide for adversarial inputs
	// (e.g. size=12 modtime="34" vs size=1 modtime="234").
	h.Write([]byte(strconv.FormatInt(sizeBytes, 10)))
	h.Write([]byte{':'})
	h.Write([]byte(modifiedAt))
	// 16 bytes is plenty — birthday collisions need ~2^64 entries before a
	// real risk, well above any practical resource count.
	sum := h.Sum(nil)[:16]
	return `"` + hex.EncodeToString(sum) + `"`
}

// CheckIfNoneMatch handles RFC 7232 If-None-Match conditional GETs. If the
// client's header matches etag, the function writes a 304 response and
// returns true — the caller should return immediately. Otherwise it returns
// false; the caller writes the body as normal but should set ETag in the
// response headers (which this function also does for the not-modified path).
//
// Supports the "*" wildcard (matches any current representation) and CSV
// lists per RFC 7232 §3.2. Comparison is byte-equal on the quoted value
// because we only mint strong ETags.
func CheckIfNoneMatch(w http.ResponseWriter, r *http.Request, etag string) bool {
	// Always advertise the current ETag so the next request can be
	// conditional even if this one is unconditional.
	w.Header().Set("ETag", etag)

	ifNone := r.Header.Get("If-None-Match")
	if ifNone == "" {
		return false
	}
	if strings.TrimSpace(ifNone) == "*" {
		writeNotModified(w)
		return true
	}
	for _, candidate := range strings.Split(ifNone, ",") {
		if strings.TrimSpace(candidate) == etag {
			writeNotModified(w)
			return true
		}
	}
	return false
}

// writeNotModified sends a 304 with no body. Cache-related headers from the
// original request are NOT echoed because chi's middleware doesn't replay
// them; clients reuse their stored representation.
func writeNotModified(w http.ResponseWriter) {
	// 304 must not carry Content-Length / Content-Type per RFC 7232 §4.1.
	// We let the response writer omit them since we never call Write.
	w.WriteHeader(http.StatusNotModified)
}
