// Package events is a small publish/subscribe hub for live updates (SSE,
// MQTT, webhooks).
package events

import (
	"sync"
	"time"
)

// Event is one live update.
type Event struct {
	Type   string    `json:"type"` // state, device, notice, log, scenes, automation, settings
	Device string    `json:"device,omitempty"`
	Data   any       `json:"data,omitempty"`
	Time   time.Time `json:"time"`
}

// Hub fans out events to subscribers. Slow subscribers drop events.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// New creates a hub.
func New() *Hub { return &Hub{subs: map[chan Event]struct{}{}} }

// Subscribe returns a channel and a cancel function.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)

	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
}

// Count is the number of subscribers.
func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	return len(h.subs)
}

// Publish sends e to all subscribers without blocking.
func (h *Hub) Publish(e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
