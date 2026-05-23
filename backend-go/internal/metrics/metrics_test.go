package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewRegistersAll(t *testing.T) {
	m := New()
	if m == nil || m.Reg == nil {
		t.Fatal("New returned nil or zero-valued Metrics")
	}
}

func TestObserveHTTPRequestRecordsCounter(t *testing.T) {
	m := New()
	m.ObserveHTTPRequest("GET", "/api/v2/files", "2xx", 0.012)
	m.ObserveHTTPRequest("GET", "/api/v2/files", "2xx", 0.018)

	got := testutil.ToFloat64(m.RequestsTotal.WithLabelValues("GET", "/api/v2/files", "2xx"))
	if got != 2 {
		t.Errorf("RequestsTotal = %v, want 2", got)
	}
}

func TestStatusClass(t *testing.T) {
	cases := map[int]string{100: "1xx", 200: "2xx", 301: "3xx", 404: "4xx", 500: "5xx", 599: "5xx"}
	for status, want := range cases {
		if got := StatusClass(status); got != want {
			t.Errorf("StatusClass(%d) = %q, want %q", status, got, want)
		}
	}
}

func TestNilReceiverIsSafe(t *testing.T) {
	var m *Metrics
	// Should not panic.
	m.ObserveHTTPRequest("GET", "/", "2xx", 0.001)
}

// TestRegistryGathersOurMetrics verifies the named series we expect show up
// when Reg is gathered — guards against future renames silently dropping the
// metric from /metrics output.
func TestRegistryGathersOurMetrics(t *testing.T) {
	m := New()
	// Bump every series at least once — Vec metrics without observed labels
	// don't surface in Gather() output.
	m.ObserveHTTPRequest("GET", "/api/v2/files", "2xx", 0.005)
	m.QueueDepth.WithLabelValues("q:thumb").Set(0)
	m.WSMessagesTotal.WithLabelValues("out", "user").Inc()

	families, err := m.Reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	wantPrefixes := []string{
		"mycloud_http_requests_total",
		"mycloud_http_request_duration_seconds",
		"mycloud_db_stmt_cache_size",
		"mycloud_queue_depth",
		"mycloud_ws_connections",
	}
	seen := make(map[string]bool)
	for _, fam := range families {
		seen[*fam.Name] = true
	}
	for _, prefix := range wantPrefixes {
		found := false
		for name := range seen {
			if strings.HasPrefix(name, prefix) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("registry missing metric with prefix %q", prefix)
		}
	}
}
