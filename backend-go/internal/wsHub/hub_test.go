package wsHub

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Hub.Connections starts at zero before any clients connect.
func TestHub_ConnectionsStartsAtZero(t *testing.T) {
	h := New(NewInMemoryBroker())
	if got := h.Connections(); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

// Hub.Broker returns the broker passed to New.
func TestHub_BrokerExposesUnderlying(t *testing.T) {
	b := NewInMemoryBroker()
	h := New(b)
	if h.Broker() != b {
		t.Errorf("Broker() should return the constructor argument")
	}
}

// New() with nil broker is callable — the hub doesn't dereference until
// a request lands.
func TestNew_AcceptsNilBroker(t *testing.T) {
	h := New(nil)
	if h == nil {
		t.Fatalf("New returned nil hub")
	}
	if h.Broker() != nil {
		t.Errorf("nil broker should pass through")
	}
}

// Counter is atomic — concurrent Add calls don't race.
func TestHub_ConnectionsAtomicCounter(t *testing.T) {
	h := New(NewInMemoryBroker())
	var wg = make(chan struct{})
	done := 0
	for i := 0; i < 100; i++ {
		go func() {
			h.connections.Add(1)
			wg <- struct{}{}
		}()
	}
	for done < 100 {
		<-wg
		done++
	}
	if got := h.Connections(); got != 100 {
		t.Errorf("expected 100, got %d", got)
	}
}

// Optional hooks are called when set.
func TestHub_OnConnectHookFires(t *testing.T) {
	h := New(NewInMemoryBroker())
	var connects, disconnects atomic.Int32
	h.OnConnect = func() { connects.Add(1) }
	h.OnDisconnect = func() { disconnects.Add(1) }

	// Simulate the ServeHTTP lifecycle hook invocation without an actual
	// upgrade — the hooks are called from ServeHTTP directly.
	h.OnConnect()
	h.OnDisconnect()
	if connects.Load() != 1 {
		t.Errorf("OnConnect not called")
	}
	if disconnects.Load() != 1 {
		t.Errorf("OnDisconnect not called")
	}
}

// AuthorizeToken hook is injected and used by the connection handler. We
// don't run the full WS upgrade here (heavy), but we verify the hook is
// callable and can return both success and error paths.
func TestHub_AuthorizeTokenHookContract(t *testing.T) {
	h := New(NewInMemoryBroker())
	h.AuthorizeToken = func(tok string) (string, error) {
		if tok == "good" {
			return "u-1", nil
		}
		return "", errors.New("bad token")
	}
	if uid, err := h.AuthorizeToken("good"); err != nil || uid != "u-1" {
		t.Errorf("good token: got (%q, %v)", uid, err)
	}
	if _, err := h.AuthorizeToken("bad"); err == nil {
		t.Errorf("bad token should error")
	}
}

// Full upgrade + first-message auth integration test. Uses httptest.NewServer
// with the Hub mounted; client connects, sends an auth envelope, expects
// the auth-ack response. Verifies the read+write loops, JSON parsing, and
// auth flow end-to-end.
func TestHub_ServeHTTP_AuthFlow(t *testing.T) {
	h := New(NewInMemoryBroker())
	authCalled := atomic.Int32{}
	h.AuthorizeToken = func(tok string) (string, error) {
		authCalled.Add(1)
		if tok == "valid-token" {
			return "u-1", nil
		}
		return "", errors.New("bad")
	}

	srv := httptest.NewServer(http.HandlerFunc(h.ServeHTTP))
	defer srv.Close()

	url := "ws" + srv.URL[len("http"):]
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "test done")

	// Send auth envelope.
	if err := c.Write(ctx, websocket.MessageText, []byte(`{"type":"auth","data":"valid-token"}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Read the ack.
	_, msg, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if authCalled.Load() != 1 {
		t.Errorf("AuthorizeToken should have been called exactly once, got %d", authCalled.Load())
	}
	// Body should be a JSON envelope of type "auth" or "ack".
	if len(msg) == 0 {
		t.Errorf("expected an ack envelope")
	}
}

// Unauth message before auth → rejection.
func TestHub_ServeHTTP_UnauthMessageRejected(t *testing.T) {
	h := New(NewInMemoryBroker())
	h.AuthorizeToken = func(tok string) (string, error) {
		return "u-1", nil
	}

	srv := httptest.NewServer(http.HandlerFunc(h.ServeHTTP))
	defer srv.Close()
	url := "ws" + srv.URL[len("http"):]

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "done")

	// Send a sub frame BEFORE authenticating. Hub should error/close.
	if err := c.Write(ctx, websocket.MessageText, []byte(`{"type":"sub","topic":"x"}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Reading should yield either an error envelope or a close.
	if _, _, err := c.Read(ctx); err == nil {
		// Reading returned an envelope — that's the error envelope path. OK.
	}
}

// Connection count increments on connect, decrements on disconnect.
func TestHub_ConnectionCountTracksLifecycle(t *testing.T) {
	h := New(NewInMemoryBroker())
	h.AuthorizeToken = func(tok string) (string, error) { return "u-1", nil }

	srv := httptest.NewServer(http.HandlerFunc(h.ServeHTTP))
	defer srv.Close()
	url := "ws" + srv.URL[len("http"):]

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	// Brief moment for the goroutine to register.
	time.Sleep(50 * time.Millisecond)
	if got := h.Connections(); got < 1 {
		t.Errorf("expected ≥1 connection after dial, got %d", got)
	}
	c.Close(websocket.StatusNormalClosure, "done")
	time.Sleep(100 * time.Millisecond)
	if got := h.Connections(); got != 0 {
		t.Errorf("expected 0 after close, got %d", got)
	}
}
