// Package wsHub owns the WebSocket pub/sub layer used to push live events
// (activity feed, slow-query stream, sync notifications) to connected
// clients. The hub is single-instance by default; the Broker interface
// abstracts the underlying transport so a Redis pub/sub adapter can drop in
// when horizontal scale ships, without rewriting handlers.
//
// Wire format (see Envelope): {type, topic, ts, data}. Topics are
// namespaced: "user:<id>", "folder:<id>", "admin:slow-queries", etc.
package wsHub

import (
	"context"
	"sync"
)

// Envelope is the over-the-wire shape every WS frame carries. JSON-encoded
// in either direction; clients send envelopes to sub/unsub, server sends
// them to push events.
type Envelope struct {
	Type  string          `json:"type"`            // "auth" | "sub" | "unsub" | "event" | "ping" | "pong" | "error" | "resync"
	Topic string          `json:"topic,omitempty"` // namespace:id
	TS    int64           `json:"ts,omitempty"`    // unix-millis when the envelope was produced
	Data  any             `json:"data,omitempty"`
	Error string          `json:"error,omitempty"` // populated on type=error
}

// Broker is the pub/sub backplane. Publish fans out a message to every local
// subscriber of topic; Subscribe/Unsubscribe register/deregister a callback.
//
// The in-memory broker is goroutine-safe. A future Redis broker would mirror
// publishes onto a Redis channel and run a goroutine consuming that channel
// to deliver messages received from other instances; subscribers don't care
// about the underlying transport.
type Broker interface {
	Publish(ctx context.Context, topic string, env Envelope) error
	Subscribe(topic string, fn func(Envelope)) (unsubscribe func())
}

// inMemoryBroker is a topic→subscribers map. Reads use an RWMutex so the
// hot Publish path doesn't serialise on adds/removes.
type inMemoryBroker struct {
	mu   sync.RWMutex
	subs map[string]map[*subscription]struct{}
}

type subscription struct {
	fn func(Envelope)
}

// NewInMemoryBroker constructs a process-local broker. Sufficient for
// single-instance deployments; replace with NewRedisBroker for multi-instance.
func NewInMemoryBroker() Broker {
	return &inMemoryBroker{subs: make(map[string]map[*subscription]struct{})}
}

func (b *inMemoryBroker) Publish(_ context.Context, topic string, env Envelope) error {
	b.mu.RLock()
	subs := b.subs[topic]
	// Snapshot the receivers under the read lock so we can release it before
	// invoking callbacks. Callbacks may take time (network sends) and we
	// don't want them blocking subscription churn.
	receivers := make([]*subscription, 0, len(subs))
	for s := range subs {
		receivers = append(receivers, s)
	}
	b.mu.RUnlock()
	for _, s := range receivers {
		s.fn(env)
	}
	return nil
}

func (b *inMemoryBroker) Subscribe(topic string, fn func(Envelope)) func() {
	s := &subscription{fn: fn}
	b.mu.Lock()
	if b.subs[topic] == nil {
		b.subs[topic] = make(map[*subscription]struct{})
	}
	b.subs[topic][s] = struct{}{}
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		if topicSet, ok := b.subs[topic]; ok {
			delete(topicSet, s)
			if len(topicSet) == 0 {
				delete(b.subs, topic)
			}
		}
		b.mu.Unlock()
	}
}
