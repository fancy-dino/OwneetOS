// SPDX-License-Identifier: GPL-3.0-or-later

// Package events delivers daemon events (controller added, Guide pressed, Wi-Fi changed, ...)
// to every client subscribed to the event stream.
package events

import (
	"sync"
	"time"
)

// Event is one notification sent to clients. Type is a stable identifier such as
// "controller.added"; Data is any JSON-encodable value.
type Event struct {
	Type string    `json:"type"`
	Time time.Time `json:"time"`
	Data any       `json:"data,omitempty"`
}

// Broker fans events out to subscribers. A subscriber that does not keep up loses events
// instead of slowing the daemon down.
type Broker struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// NewBroker returns an empty broker.
func NewBroker() *Broker {
	return &Broker{subs: make(map[chan Event]struct{})}
}

// Subscribe returns a channel receiving every published event, and a function that ends the
// subscription. buffer is the number of events kept for a slow subscriber.
func (b *Broker) Subscribe(buffer int) (<-chan Event, func()) {
	ch := make(chan Event, buffer)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, ch)
			b.mu.Unlock()
			close(ch)
		})
	}
}

// Publish sends an event to every subscriber without blocking.
func (b *Broker) Publish(eventType string, data any) {
	ev := Event{Type: eventType, Time: time.Now().UTC(), Data: data}
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default: // subscriber too slow: drop this event for it
		}
	}
}

// Subscribers returns the number of active subscriptions.
func (b *Broker) Subscribers() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}
