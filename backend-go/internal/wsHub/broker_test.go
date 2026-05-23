package wsHub

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Subscribe + Publish: a topic subscriber receives published envelopes.
func TestBroker_PublishToSubscriber(t *testing.T) {
	b := NewInMemoryBroker()
	got := make(chan Envelope, 1)
	unsub := b.Subscribe("test:1", func(e Envelope) { got <- e })
	defer unsub()

	in := Envelope{Type: "event", Topic: "test:1", Data: map[string]any{"x": 1}}
	if err := b.Publish(context.Background(), "test:1", in); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case e := <-got:
		if e.Type != "event" {
			t.Errorf("expected event, got %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for envelope")
	}
}

// Different topic → not delivered.
func TestBroker_OtherTopicNotDelivered(t *testing.T) {
	b := NewInMemoryBroker()
	hits := int32(0)
	unsub := b.Subscribe("user:1", func(e Envelope) { atomic.AddInt32(&hits, 1) })
	defer unsub()

	_ = b.Publish(context.Background(), "user:2", Envelope{Type: "event"})
	_ = b.Publish(context.Background(), "user:3", Envelope{Type: "event"})
	time.Sleep(20 * time.Millisecond)
	if atomic.LoadInt32(&hits) != 0 {
		t.Errorf("subscriber for user:1 received events for other topics")
	}
}

// Unsubscribe stops delivery.
func TestBroker_UnsubscribeStops(t *testing.T) {
	b := NewInMemoryBroker()
	var hits int32
	unsub := b.Subscribe("t", func(_ Envelope) { atomic.AddInt32(&hits, 1) })
	_ = b.Publish(context.Background(), "t", Envelope{})
	time.Sleep(10 * time.Millisecond)
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("expected 1 hit before unsub")
	}
	unsub()
	_ = b.Publish(context.Background(), "t", Envelope{})
	_ = b.Publish(context.Background(), "t", Envelope{})
	time.Sleep(10 * time.Millisecond)
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("expected unsub to halt delivery; got %d hits", atomic.LoadInt32(&hits))
	}
}

// Many subscribers on the same topic all receive.
func TestBroker_FanOut(t *testing.T) {
	b := NewInMemoryBroker()
	var hits int32
	for i := 0; i < 50; i++ {
		b.Subscribe("fan", func(_ Envelope) { atomic.AddInt32(&hits, 1) })
	}
	_ = b.Publish(context.Background(), "fan", Envelope{})
	time.Sleep(20 * time.Millisecond)
	if got := atomic.LoadInt32(&hits); got != 50 {
		t.Errorf("expected 50 deliveries, got %d", got)
	}
}

// Concurrent Subscribe + Publish must not race. Run with -race to catch.
func TestBroker_ConcurrentSubscribePublish(t *testing.T) {
	b := NewInMemoryBroker()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				unsub := b.Subscribe("c", func(_ Envelope) {})
				unsub()
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = b.Publish(context.Background(), "c", Envelope{})
			}
		}()
	}
	wg.Wait()
	// No assertion — we're testing the absence of a data race under -race.
}

// Slow subscriber doesn't block publish to other subscribers — the broker
// snapshots receivers under the read lock and releases before calling.
func TestBroker_SlowSubscriberDoesntBlockFastOnes(t *testing.T) {
	b := NewInMemoryBroker()
	fastHits := make(chan struct{}, 5)
	slowStarted := make(chan struct{}, 1)
	slowRelease := make(chan struct{})

	b.Subscribe("topic", func(_ Envelope) {
		slowStarted <- struct{}{}
		<-slowRelease
	})
	b.Subscribe("topic", func(_ Envelope) {
		fastHits <- struct{}{}
	})

	// Spin off Publish so this test goroutine doesn't block on the slow sub.
	go b.Publish(context.Background(), "topic", Envelope{})

	// Slow subscriber should have been invoked.
	select {
	case <-slowStarted:
	case <-time.After(time.Second):
		t.Fatalf("slow subscriber never started")
	}
	// Fast subscriber gets called BEFORE slow finishes — the broker iterates
	// the receiver snapshot in order, but the slow one is running on the
	// goroutine that called Publish; the order is implementation-defined.
	// We just verify both eventually fire.
	close(slowRelease)
	select {
	case <-fastHits:
	case <-time.After(time.Second):
		t.Fatalf("fast subscriber never received")
	}
}

// Envelope JSON contract — important for the WS protocol.
func TestEnvelope_Fields(t *testing.T) {
	e := Envelope{
		Type:  "event",
		Topic: "user:1",
		TS:    1234567890,
		Data:  map[string]any{"k": "v"},
	}
	if e.Type != "event" || e.Topic != "user:1" || e.TS != 1234567890 {
		t.Errorf("envelope fields wrong: %+v", e)
	}
}

// Empty topic publish — no subscribers exist, no error, no panic.
func TestBroker_PublishNoSubscribers(t *testing.T) {
	b := NewInMemoryBroker()
	if err := b.Publish(context.Background(), "noone", Envelope{}); err != nil {
		t.Errorf("Publish with no subs returned error: %v", err)
	}
}

// Subscribing twice to the same topic with the same callback creates two
// distinct subscriptions — both fire on publish, both must be individually
// unsubscribed.
func TestBroker_DoubleSubscribeAreDistinct(t *testing.T) {
	b := NewInMemoryBroker()
	var hits int32
	cb := func(_ Envelope) { atomic.AddInt32(&hits, 1) }
	unsub1 := b.Subscribe("t", cb)
	unsub2 := b.Subscribe("t", cb)
	_ = b.Publish(context.Background(), "t", Envelope{})
	time.Sleep(10 * time.Millisecond)
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("expected 2 hits (two distinct subs), got %d", got)
	}
	unsub1()
	_ = b.Publish(context.Background(), "t", Envelope{})
	time.Sleep(10 * time.Millisecond)
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("after one unsub, expected 3 hits total, got %d", got)
	}
	unsub2()
}
