package sse

import (
	"sync"

	"github.com/google/uuid"
)

// Hub defines the contract for real-time progress event pub/sub.
type Hub interface {
	Subscribe(recordingID uuid.UUID) (<-chan ProgressEvent, func())
	Publish(recordingID uuid.UUID, event ProgressEvent)
	SubscriberCount(recordingID uuid.UUID) int
	Close()
}

// InMemHub is a thread-safe in-memory pub/sub broker for SSE stream clients.
type InMemHub struct {
	mu          sync.RWMutex
	subscribers map[uuid.UUID]map[chan ProgressEvent]struct{}
	closed      bool
}

// NewHub initializes a new InMemHub.
func NewHub() *InMemHub {
	return &InMemHub{
		subscribers: make(map[uuid.UUID]map[chan ProgressEvent]struct{}),
	}
}

// Subscribe attaches a listener channel for the given recording ID.
// Returns the read-only channel and an unsubscribe closure.
func (h *InMemHub) Subscribe(recordingID uuid.UUID) (<-chan ProgressEvent, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan ProgressEvent, 16)
	if h.closed {
		close(ch)
		return ch, func() {}
	}

	if _, ok := h.subscribers[recordingID]; !ok {
		h.subscribers[recordingID] = make(map[chan ProgressEvent]struct{})
	}
	h.subscribers[recordingID][ch] = struct{}{}

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()

			if subs, ok := h.subscribers[recordingID]; ok {
				delete(subs, ch)
				if len(subs) == 0 {
					delete(h.subscribers, recordingID)
				}
			}
			close(ch)
		})
	}

	return ch, unsubscribe
}

// Publish broadcasts a ProgressEvent to all active subscribers of the recording.
func (h *InMemHub) Publish(recordingID uuid.UUID, event ProgressEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.closed {
		return
	}

	subs, ok := h.subscribers[recordingID]
	if !ok || len(subs) == 0 {
		return
	}

	for ch := range subs {
		select {
		case ch <- event:
		default:
			// Non-blocking drop if channel buffer is full to prevent pipeline worker stalling
		}
	}
}

// SubscriberCount returns current active listener count for a recording.
func (h *InMemHub) SubscriberCount(recordingID uuid.UUID) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.closed {
		return 0
	}

	if subs, ok := h.subscribers[recordingID]; ok {
		return len(subs)
	}
	return 0
}

// Close shuts down the hub and cleanly unsubscribes all active listeners.
func (h *InMemHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}
	h.closed = true

	for recID, subs := range h.subscribers {
		for ch := range subs {
			close(ch)
		}
		delete(h.subscribers, recID)
	}
}
