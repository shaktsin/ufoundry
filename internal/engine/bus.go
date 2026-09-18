package engine

import (
	"sync"
)

// Subscriber receives engine notifications. Implementations must not block:
// the server drops messages for clients whose buffers are full.
type Subscriber interface {
	ID() string
	// Admin clients receive approval requests.
	IsAdmin() bool
	// Wants reports whether the subscriber follows this thread.
	Wants(threadID string) bool
	Notify(method string, params any)
}

// Bus fans notifications out to subscribers.
type Bus struct {
	mu   sync.RWMutex
	subs map[string]Subscriber
}

// NewBus returns an empty bus.
func NewBus() *Bus { return &Bus{subs: map[string]Subscriber{}} }

// Add registers a subscriber.
func (b *Bus) Add(s Subscriber) {
	b.mu.Lock()
	b.subs[s.ID()] = s
	b.mu.Unlock()
}

// Remove unregisters a subscriber.
func (b *Bus) Remove(id string) {
	b.mu.Lock()
	delete(b.subs, id)
	b.mu.Unlock()
}

// Count returns the number of subscribers.
func (b *Bus) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}

// Publish sends a thread-scoped notification to subscribers following threadID.
func (b *Bus) Publish(threadID, method string, params any) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, s := range b.subs {
		if s.Wants(threadID) {
			s.Notify(method, params)
		}
	}
}

// PublishAdmin sends a notification to every admin subscriber.
func (b *Bus) PublishAdmin(method string, params any) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, s := range b.subs {
		if s.IsAdmin() {
			s.Notify(method, params)
		}
	}
}
