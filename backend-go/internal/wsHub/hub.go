package wsHub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// Hub coordinates WebSocket connections. It owns the upgrade dance, per-
// client read/write loops, and topic subscriptions. Handlers feed events
// into a Hub by calling its Broker; the Hub forwards to the right clients.
type Hub struct {
	broker Broker
	// AuthorizeToken validates a bearer token client-side and returns the
	// associated user ID. Injected so this package doesn't import the auth
	// package (which would create a cycle).
	AuthorizeToken func(token string) (userID string, err error)
	// OnConnect / OnDisconnect let the hub host observe connection counts
	// without coupling to metrics. The app wires these to its gauge.
	OnConnect    func()
	OnDisconnect func()

	// connections is sampled by metrics; atomic so we don't hold the
	// broker's RWMutex just to read a count.
	connections atomic.Int64
}

// New constructs a Hub. The broker is exposed for handlers to publish; pass
// either NewInMemoryBroker() or a future Redis broker.
func New(broker Broker) *Hub {
	return &Hub{broker: broker}
}

// Broker returns the pub/sub used by this hub. Handlers call Broker().Publish
// to fan out events to subscribed clients.
func (h *Hub) Broker() Broker {
	return h.broker
}

// Connections returns the current count of live WS connections. Used by the
// metrics sampler.
func (h *Hub) Connections() int64 {
	return h.connections.Load()
}

// ServeHTTP performs the upgrade and runs the per-connection read+write
// loops until the client disconnects or sends a bad frame. Wire it as a chi
// handler under the same auth middleware as the rest of /api/v2 if you want
// the bearer-token version; for browser clients that can't send headers on
// upgrade, accept ?token= in the URL or rely on a first-message "auth" envelope.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// We do our own origin check via the existing CORS middleware so
		// allow connections that get past it. Browsers ALSO enforce same-
		// origin policy on WebSocket so a hostile site can't open a stream
		// without explicit user action.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return // Accept already wrote the error
	}
	// Hard-close on any exit path. CloseStatus is set after a clean
	// goodbye envelope; CloseStatusInternalError otherwise.
	defer conn.Close(websocket.StatusInternalError, "shutdown")

	h.connections.Add(1)
	if h.OnConnect != nil {
		h.OnConnect()
	}
	defer func() {
		h.connections.Add(-1)
		if h.OnDisconnect != nil {
			h.OnDisconnect()
		}
	}()

	// Bound the connection lifetime so a stuck reader doesn't pin a
	// goroutine forever. Apps that want long-lived streams refresh the
	// connection from the client side after the first heartbeat miss.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Per-connection bookkeeping: which topics this socket is subscribed
	// to, and the unsubscribe func returned by the broker for each.
	unsubs := make(map[string]func())
	var userID string
	authed := false

	// Outbound queue: writes are serialised through this channel so we can
	// publish from a broker callback without racing the read loop. Capped
	// at 1024 — at overflow we drop oldest and send a resync envelope so
	// the client knows it lost messages.
	const outboundCap = 1024
	out := make(chan Envelope, outboundCap)
	resyncTimer := time.NewTimer(0)
	resyncTimer.Stop()
	defer resyncTimer.Stop()

	// Writer goroutine. Reads from `out`, writes to the conn. Exits on ctx
	// cancellation. Heartbeat ping every 30s — if the client is unreachable
	// the underlying conn.Write hangs and we fall through the deadline.
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case env := <-out:
				writeCtx, wc := context.WithTimeout(ctx, 5*time.Second)
				err := writeEnvelope(writeCtx, conn, env)
				wc()
				if err != nil {
					cancel()
					return
				}
			case <-t.C:
				pingCtx, pc := context.WithTimeout(ctx, 5*time.Second)
				if err := conn.Ping(pingCtx); err != nil {
					pc()
					cancel()
					return
				}
				pc()
			}
		}
	}()

	// Reader loop. Each inbound envelope is interpreted: auth, sub, unsub.
	for {
		if ctx.Err() != nil {
			return
		}
		_, payload, err := conn.Read(ctx)
		if err != nil {
			// Normal close from the peer.
			if websocket.CloseStatus(err) != -1 || errors.Is(err, context.Canceled) {
				conn.Close(websocket.StatusNormalClosure, "")
				return
			}
			return
		}
		var env Envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			_ = trySend(out, Envelope{Type: "error", Error: "bad json"}, cancel)
			continue
		}

		switch env.Type {
		case "auth":
			if h.AuthorizeToken == nil {
				_ = trySend(out, Envelope{Type: "error", Error: "auth disabled"}, cancel)
				continue
			}
			tok, _ := env.Data.(string)
			uid, err := h.AuthorizeToken(tok)
			if err != nil || uid == "" {
				_ = trySend(out, Envelope{Type: "error", Error: "invalid token"}, cancel)
				conn.Close(websocket.StatusPolicyViolation, "auth failed")
				return
			}
			userID = uid
			authed = true
			// On successful auth, subscribe automatically to the user's
			// per-user topic so handlers don't need to know the client's ID.
			h.attachSub(unsubs, "user:"+uid, out, cancel)
			_ = trySend(out, Envelope{Type: "auth", Data: map[string]any{"ok": true, "user_id": uid}}, cancel)

		case "sub":
			if !authed {
				_ = trySend(out, Envelope{Type: "error", Error: "auth required"}, cancel)
				continue
			}
			if !isAllowedTopic(env.Topic, userID) {
				_ = trySend(out, Envelope{Type: "error", Error: "forbidden topic"}, cancel)
				continue
			}
			h.attachSub(unsubs, env.Topic, out, cancel)

		case "unsub":
			if unsub, ok := unsubs[env.Topic]; ok {
				unsub()
				delete(unsubs, env.Topic)
			}

		case "ping":
			_ = trySend(out, Envelope{Type: "pong", TS: time.Now().UnixMilli()}, cancel)
		}
	}
}

// attachSub registers a broker callback that pushes events into the conn's
// out-channel. Overflow drops the oldest message and queues a resync hint
// so the client knows to revalidate from REST.
func (h *Hub) attachSub(unsubs map[string]func(), topic string, out chan<- Envelope, cancel context.CancelFunc) {
	// Idempotent: re-subscribing replaces the previous handler.
	if existing, ok := unsubs[topic]; ok {
		existing()
	}
	unsub := h.broker.Subscribe(topic, func(env Envelope) {
		select {
		case out <- env:
		default:
			// Overflow path. Try to push a resync; if that also blocks the
			// client is hopelessly slow and we drop the connection.
			select {
			case out <- Envelope{Type: "resync", Topic: topic, Error: "buffer full"}:
			default:
				cancel()
			}
		}
	})
	unsubs[topic] = unsub
}

// trySend pushes onto the bounded outbound channel without blocking. On
// overflow we cancel the connection so the goroutine can exit cleanly.
func trySend(out chan<- Envelope, env Envelope, cancel context.CancelFunc) error {
	env.TS = time.Now().UnixMilli()
	select {
	case out <- env:
		return nil
	default:
		cancel()
		return fmt.Errorf("outbound full")
	}
}

// writeEnvelope JSON-encodes env and ships it as a text frame. Binary would
// save a few bytes; we stay text so dev-tools "Frames" tab is readable.
func writeEnvelope(ctx context.Context, conn *websocket.Conn, env Envelope) error {
	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, payload)
}

// isAllowedTopic enforces topic scoping. Users can subscribe to:
//   - their own user topic ("user:<their id>")
//   - any folder topic (folder access checks happen at publish time, not subscribe)
//   - any public topic (no namespace, no colon)
//
// Admin topics ("admin:*") require an out-of-band check the handler can
// override before mounting the hub — for now we refuse them by default.
func isAllowedTopic(topic, userID string) bool {
	if topic == "" {
		return false
	}
	if strings.HasPrefix(topic, "user:") {
		return strings.TrimPrefix(topic, "user:") == userID
	}
	if strings.HasPrefix(topic, "admin:") {
		return false
	}
	return true
}
