// SPDX-License-Identifier: GPL-3.0-or-later

package events

import (
	"testing"
	"time"
)

func TestPublishReachesSubscribers(t *testing.T) {
	b := NewBroker()
	a, cancelA := b.Subscribe(4)
	c, cancelC := b.Subscribe(4)
	defer cancelA()
	defer cancelC()

	b.Publish("guide.pressed", map[string]string{"controller": "1"})

	for _, ch := range []<-chan Event{a, c} {
		select {
		case ev := <-ch:
			if ev.Type != "guide.pressed" {
				t.Fatalf("got type %q", ev.Type)
			}
		case <-time.After(time.Second):
			t.Fatal("event not delivered")
		}
	}
}

func TestSlowSubscriberDoesNotBlock(t *testing.T) {
	b := NewBroker()
	_, cancel := b.Subscribe(1)
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			b.Publish("audio.changed", i)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}
}

func TestUnsubscribe(t *testing.T) {
	b := NewBroker()
	ch, cancel := b.Subscribe(1)
	cancel()
	cancel() // second call must be harmless
	if b.Subscribers() != 0 {
		t.Fatalf("still %d subscribers", b.Subscribers())
	}
	if _, open := <-ch; open {
		t.Fatal("channel not closed")
	}
}
