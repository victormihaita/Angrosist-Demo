package broker

import (
	"testing"
	"time"

	"github.com/angrosist/demo/internal/ports"
)

// recv reads one event with a timeout so a hung test fails fast.
func recv(t *testing.T, ch <-chan ports.Event) (ports.Event, bool) {
	t.Helper()
	select {
	case ev, ok := <-ch:
		return ev, ok
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return ports.Event{}, false
	}
}

// TestSubscribeReceivesPublished asserts a subscriber receives a published event.
func TestSubscribeReceivesPublished(t *testing.T) {
	b := NewInProcess()
	ch, unsub := b.Subscribe("conv-1")
	defer unsub()

	b.Publish("conv-1", ports.Event{Type: ports.EventMessage, Reply: "hi"})

	ev, ok := recv(t, ch)
	if !ok {
		t.Fatal("channel closed unexpectedly")
	}
	if ev.Type != ports.EventMessage || ev.Reply != "hi" {
		t.Fatalf("unexpected event: %+v", ev)
	}
}

// TestUnsubscribeStopsDelivery asserts no events arrive after unsubscribe and the
// channel is closed.
func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := NewInProcess()
	ch, unsub := b.Subscribe("conv-1")

	unsub()

	// The channel must be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel after unsubscribe")
		}
	case <-time.After(time.Second):
		t.Fatal("expected channel to be closed")
	}

	// Publishing after unsubscribe must not panic (no send on closed channel).
	b.Publish("conv-1", ports.Event{Type: ports.EventTyping})

	// Unsubscribe is idempotent.
	unsub()
}

// TestMultipleSubscribersSameConversation asserts each subscriber to the same
// conversation receives the event.
func TestMultipleSubscribersSameConversation(t *testing.T) {
	b := NewInProcess()
	ch1, unsub1 := b.Subscribe("conv-1")
	defer unsub1()
	ch2, unsub2 := b.Subscribe("conv-1")
	defer unsub2()

	b.Publish("conv-1", ports.Event{Type: ports.EventTyping})

	if ev, _ := recv(t, ch1); ev.Type != ports.EventTyping {
		t.Fatalf("sub1: unexpected event: %+v", ev)
	}
	if ev, _ := recv(t, ch2); ev.Type != ports.EventTyping {
		t.Fatalf("sub2: unexpected event: %+v", ev)
	}
}

// TestPublishNoSubscribersIsNoOp asserts publishing to a conversation with no
// subscribers does nothing and does not panic.
func TestPublishNoSubscribersIsNoOp(t *testing.T) {
	b := NewInProcess()
	b.Publish("nobody", ports.Event{Type: ports.EventMessage, Reply: "x"})
}

// TestSlowSubscriberDoesNotBlockPublish asserts a full subscriber buffer never
// blocks Publish: events past the buffer are dropped for that subscriber.
func TestSlowSubscriberDoesNotBlockPublish(t *testing.T) {
	b := NewInProcess()
	ch, unsub := b.Subscribe("conv-1")
	defer unsub()

	// Publish far more than the buffer without ever reading. If Publish blocked,
	// this would deadlock and the test would time out.
	done := make(chan struct{})
	go func() {
		for i := 0; i < subBufferSize*4; i++ {
			b.Publish("conv-1", ports.Event{Type: ports.EventTyping})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}

	// The buffered events that fit are still readable.
	if ev, _ := recv(t, ch); ev.Type != ports.EventTyping {
		t.Fatalf("unexpected event: %+v", ev)
	}
}
