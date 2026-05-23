package app

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// goSafe must catch panics from the launched goroutine so a single worker
// failure doesn't kill the whole process. Sync via WaitGroup since the
// panic-recovering goroutine still needs to return cleanly.
func TestGoSafe_RecoversPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	goSafe("test-worker", func() {
		defer wg.Done()
		panic("intentional")
	})
	wg.Wait() // would block forever if the panic killed the goroutine before Done()
}

// Non-panic functions run to completion exactly once.
func TestGoSafe_NormalRunCallsOnce(t *testing.T) {
	var count int32
	var wg sync.WaitGroup
	wg.Add(1)
	goSafe("normal", func() {
		defer wg.Done()
		atomic.AddInt32(&count, 1)
	})
	wg.Wait()
	if got := atomic.LoadInt32(&count); got != 1 {
		t.Errorf("expected fn called exactly once, got %d", got)
	}
}

// Multiple panics in distinct goroutines stay isolated — one bad job
// doesn't poison its siblings.
func TestGoSafe_ConcurrentPanicsIsolated(t *testing.T) {
	var ok int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		i := i
		goSafe("concurrent", func() {
			defer wg.Done()
			if i%2 == 0 {
				panic("odd one out")
			}
			atomic.AddInt32(&ok, 1)
		})
	}
	wg.Wait()
	if got := atomic.LoadInt32(&ok); got != 10 {
		t.Errorf("expected 10 non-panicking goroutines to complete, got %d", got)
	}
}

// isIndexableMime is consulted by afterUploadCommit to decide whether to
// enqueue a content-extraction job. Test the contract: text and select
// document types yes, everything else no.
func TestIsIndexableMime(t *testing.T) {
	yes := []string{
		"text/plain",
		"text/csv",
		"text/markdown",
		"application/pdf",
		"application/msword",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.ms-excel",
	}
	no := []string{
		"image/jpeg",
		"video/mp4",
		"application/zip",
		"audio/mpeg",
		"application/octet-stream",
		"",
	}
	for _, m := range yes {
		if !isIndexableMime(m) {
			t.Errorf("expected indexable: %q", m)
		}
	}
	for _, m := range no {
		if isIndexableMime(strings.ToLower(m)) {
			t.Errorf("expected not indexable: %q", m)
		}
	}
}

// deviceLabelFromUA produces a short human label for the session list.
// Test browser + platform detection covers the common cases without
// pulling in a full UA-parsing dependency.
func TestDeviceLabelFromUA(t *testing.T) {
	cases := []struct {
		name string
		ua   string
		want string
	}{
		{"chrome on mac",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.6167.85 Safari/537.36",
			"Chrome on macOS"},
		{"firefox on windows",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Gecko/20100101 Firefox/124.0",
			"Firefox on Windows"},
		{"safari on iphone",
			"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605 Version/17.0 Mobile/15E148 Safari/604.1",
			"Safari on iPhone"},
		{"edge on windows",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Edg/121.0.0.0",
			"Edge on Windows"},
		{"chrome on android",
			"Mozilla/5.0 (Linux; Android 14) Chrome/121.0.0.0 Mobile Safari/537.36",
			"Chrome on Android"},
		{"curl",
			"curl/8.4.0",
			"curl on Unknown OS"},
		{"empty UA",
			"",
			"Unknown browser on Unknown OS"},
		{"electron desktop sync",
			"MyCloud-Desktop/1.0.0 Electron/27.0",
			"Desktop sync on Unknown OS"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := deviceLabelFromUA(c.ua)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
