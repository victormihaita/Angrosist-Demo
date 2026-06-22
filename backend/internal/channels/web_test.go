package channels

import (
	"context"
	"testing"

	"github.com/angrosist/demo/internal/broker"
	"github.com/angrosist/demo/internal/domain"
	"github.com/angrosist/demo/internal/ports"
)

func TestWeb_PublishesLifecycleToBroker(t *testing.T) {
	b := broker.NewInProcess()
	ch, unsub := b.Subscribe("conv-1")
	defer unsub()

	web := NewWeb(b)
	conv := &domain.Conversation{ID: "conv-1", Channel: ChannelWeb, State: domain.StateQualifying}

	if err := web.Typing(context.Background(), conv); err != nil {
		t.Fatalf("typing: %v", err)
	}
	if err := web.Deliver(context.Background(), conv, ports.Event{Type: ports.EventMessage, Reply: "salut", State: "qualifying"}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if err := web.Error(context.Background(), conv, "agent error"); err != nil {
		t.Fatalf("error: %v", err)
	}

	want := []struct {
		typ   string
		reply string
		errm  string
	}{
		{ports.EventTyping, "", ""},
		{ports.EventMessage, "salut", ""},
		{ports.EventError, "", "agent error"},
	}
	for i, exp := range want {
		ev := <-ch
		if ev.Type != exp.typ || ev.Reply != exp.reply || ev.Error != exp.errm {
			t.Fatalf("event %d = %+v, want type=%s reply=%q error=%q", i, ev, exp.typ, exp.reply, exp.errm)
		}
	}
}

func TestWeb_NilBrokerIsNoop(t *testing.T) {
	web := NewWeb(nil)
	conv := &domain.Conversation{ID: "c", Channel: ChannelWeb}
	if err := web.Deliver(context.Background(), conv, ports.Event{Reply: "x"}); err != nil {
		t.Fatalf("deliver with nil broker should be no-op: %v", err)
	}
}

func TestRegistry_ResolvesAndFallsBack(t *testing.T) {
	web := NewWeb(nil)
	reg := NewRegistry(map[string]ports.Replier{ChannelWeb: web})

	if reg.For(ChannelWeb) != web {
		t.Fatal("expected web replier for web channel")
	}
	if _, ok := reg.For("unknown").(Noop); !ok {
		t.Fatal("expected Noop fallback for unknown channel")
	}
}
