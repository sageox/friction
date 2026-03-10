// Package ringbuffer provides a generic thread-safe circular buffer with deduplication.
package ringbuffer

import "sync"

// RingBuffer is a thread-safe circular buffer.
// When full, new items overwrite the oldest. Deduplicates by key within each
// flush window — if an identical item is already in the buffer, the new one
// is silently dropped.
type RingBuffer[T any] struct {
	mu       sync.Mutex
	items    []T
	capacity int
	head     int // next write position
	count    int // current number of stored items
	seen     map[string]bool
	keyFunc  func(T) string
}

// New creates a RingBuffer with the given capacity and deduplication key function.
// The keyFunc extracts a deduplication key from each item.
func New[T any](capacity int, keyFunc func(T) string) *RingBuffer[T] {
	if capacity <= 0 {
		capacity = 1
	}
	return &RingBuffer[T]{
		items:    make([]T, capacity),
		capacity: capacity,
		seen:     make(map[string]bool),
		keyFunc:  keyFunc,
	}
}

// Add inserts an item into the buffer, overwriting the oldest if full.
// Duplicate items (same key) are silently dropped.
func (rb *RingBuffer[T]) Add(item T) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	key := rb.keyFunc(item)
	if rb.seen[key] {
		return
	}

	// if overwriting an existing item, remove its dedup key
	if rb.count == rb.capacity {
		oldKey := rb.keyFunc(rb.items[rb.head])
		delete(rb.seen, oldKey)
	}

	rb.items[rb.head] = item
	rb.head = (rb.head + 1) % rb.capacity

	if rb.count < rb.capacity {
		rb.count++
	}
	rb.seen[key] = true
}

// Drain returns all items in chronological order (oldest first) and clears the buffer.
func (rb *RingBuffer[T]) Drain() []T {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 {
		return nil
	}

	result := make([]T, rb.count)

	start := 0
	if rb.count == rb.capacity {
		start = rb.head
	}

	for i := 0; i < rb.count; i++ {
		idx := (start + i) % rb.capacity
		result[i] = rb.items[idx]
	}

	rb.head = 0
	rb.count = 0
	rb.seen = make(map[string]bool)

	return result
}

// Count returns the current number of stored items.
func (rb *RingBuffer[T]) Count() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}

// Capacity returns the buffer capacity.
func (rb *RingBuffer[T]) Capacity() int {
	return rb.capacity
}
