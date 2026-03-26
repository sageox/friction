package ringbuffer

import (
	"fmt"
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name             string
		capacity         int
		expectedCapacity int
	}{
		{name: "positive capacity", capacity: 10, expectedCapacity: 10},
		{name: "capacity of one", capacity: 1, expectedCapacity: 1},
		{name: "zero capacity defaults to one", capacity: 0, expectedCapacity: 1},
		{name: "negative capacity defaults to one", capacity: -5, expectedCapacity: 1},
	}

	keyFunc := func(s string) string { return s }

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rb := New(tc.capacity, keyFunc)
			if rb == nil {
				t.Fatal("New returned nil")
			}
			if rb.capacity != tc.expectedCapacity {
				t.Errorf("capacity = %d, want %d", rb.capacity, tc.expectedCapacity)
			}
			if len(rb.items) != tc.expectedCapacity {
				t.Errorf("items slice length = %d, want %d", len(rb.items), tc.expectedCapacity)
			}
			if rb.count != 0 {
				t.Errorf("initial count = %d, want 0", rb.count)
			}
			if rb.head != 0 {
				t.Errorf("initial head = %d, want 0", rb.head)
			}
			if rb.seen == nil {
				t.Error("seen map should be initialized")
			}
		})
	}
}

func TestRingBuffer_Add(t *testing.T) {
	tests := []struct {
		name          string
		capacity      int
		eventsToAdd   int
		expectedCount int
	}{
		{name: "add single item", capacity: 5, eventsToAdd: 1, expectedCount: 1},
		{name: "add items up to capacity", capacity: 5, eventsToAdd: 5, expectedCount: 5},
		{name: "add items beyond capacity", capacity: 3, eventsToAdd: 7, expectedCount: 3},
		{name: "capacity of one with multiple adds", capacity: 1, eventsToAdd: 5, expectedCount: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rb := New(tc.capacity, func(s string) string { return s })

			for i := 0; i < tc.eventsToAdd; i++ {
				rb.Add(fmt.Sprintf("item%d", i))
			}

			if rb.Count() != tc.expectedCount {
				t.Errorf("Count() = %d, want %d", rb.Count(), tc.expectedCount)
			}
		})
	}
}

func TestRingBuffer_Dedup(t *testing.T) {
	t.Run("duplicate items are dropped", func(t *testing.T) {
		rb := New(10, func(s string) string { return s })
		rb.Add("item1")
		rb.Add("item1")
		rb.Add("item1")

		if rb.Count() != 1 {
			t.Errorf("Count() = %d, want 1 (duplicates should be dropped)", rb.Count())
		}
	})

	t.Run("different items are distinct", func(t *testing.T) {
		rb := New(10, func(s string) string { return s })
		rb.Add("item1")
		rb.Add("item2")

		if rb.Count() != 2 {
			t.Errorf("Count() = %d, want 2", rb.Count())
		}
	})

	t.Run("dedup resets after drain", func(t *testing.T) {
		rb := New(10, func(s string) string { return s })
		rb.Add("item1")

		if rb.Count() != 1 {
			t.Fatalf("Count() = %d, want 1", rb.Count())
		}

		rb.Drain()

		rb.Add("item1")
		if rb.Count() != 1 {
			t.Errorf("Count() after re-add = %d, want 1 (dedup should reset on drain)", rb.Count())
		}
	})

	t.Run("overwritten item key is freed", func(t *testing.T) {
		rb := New(2, func(s string) string { return s })

		rb.Add("cmd1")
		rb.Add("cmd2")
		// buffer full: [cmd1, cmd2]

		// cmd3 overwrites cmd1, freeing "cmd1"
		rb.Add("cmd3")

		// cmd1 should be accepted again since it was evicted
		rb.Add("cmd1")

		items := rb.Drain()
		if len(items) != 2 {
			t.Fatalf("Drain() returned %d items, want 2", len(items))
		}
		if items[0] != "cmd3" {
			t.Errorf("items[0] = %q, want %q", items[0], "cmd3")
		}
		if items[1] != "cmd1" {
			t.Errorf("items[1] = %q, want %q", items[1], "cmd1")
		}
	})
}

func TestRingBuffer_Drain(t *testing.T) {
	tests := []struct {
		name          string
		capacity      int
		itemsToAdd    []string
		expectedItems []string
	}{
		{
			name:          "drain empty buffer",
			capacity:      5,
			itemsToAdd:    []string{},
			expectedItems: nil,
		},
		{
			name:          "drain single item",
			capacity:      5,
			itemsToAdd:    []string{"cmd1"},
			expectedItems: []string{"cmd1"},
		},
		{
			name:          "drain multiple items in order",
			capacity:      5,
			itemsToAdd:    []string{"cmd1", "cmd2", "cmd3"},
			expectedItems: []string{"cmd1", "cmd2", "cmd3"},
		},
		{
			name:          "drain at capacity",
			capacity:      3,
			itemsToAdd:    []string{"cmd1", "cmd2", "cmd3"},
			expectedItems: []string{"cmd1", "cmd2", "cmd3"},
		},
		{
			name:          "drain after overwrite preserves chronological order",
			capacity:      3,
			itemsToAdd:    []string{"cmd1", "cmd2", "cmd3", "cmd4", "cmd5"},
			expectedItems: []string{"cmd3", "cmd4", "cmd5"},
		},
		{
			name:          "drain with capacity one after multiple adds",
			capacity:      1,
			itemsToAdd:    []string{"cmd1", "cmd2", "cmd3"},
			expectedItems: []string{"cmd3"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rb := New(tc.capacity, func(s string) string { return s })

			for _, item := range tc.itemsToAdd {
				rb.Add(item)
			}

			result := rb.Drain()

			if tc.expectedItems == nil {
				if result != nil {
					t.Errorf("Drain() = %v, want nil", result)
				}
				return
			}

			if len(result) != len(tc.expectedItems) {
				t.Fatalf("Drain() returned %d items, want %d", len(result), len(tc.expectedItems))
			}

			for i, expected := range tc.expectedItems {
				if result[i] != expected {
					t.Errorf("result[%d] = %q, want %q", i, result[i], expected)
				}
			}

			if rb.Count() != 0 {
				t.Errorf("Count() after Drain() = %d, want 0", rb.Count())
			}
			if rb.head != 0 {
				t.Errorf("head after Drain() = %d, want 0", rb.head)
			}
		})
	}
}

func TestRingBuffer_Drain_ClearsBuffer(t *testing.T) {
	rb := New(5, func(s string) string { return s })

	rb.Add("cmd1")
	rb.Add("cmd2")

	first := rb.Drain()
	if len(first) != 2 {
		t.Fatalf("first Drain() returned %d items, want 2", len(first))
	}

	second := rb.Drain()
	if second != nil {
		t.Errorf("second Drain() = %v, want nil", second)
	}
}

func TestRingBuffer_Count(t *testing.T) {
	rb := New(5, func(s string) string { return s })

	if rb.Count() != 0 {
		t.Errorf("Count() on empty buffer = %d, want 0", rb.Count())
	}

	rb.Add("cmd1")
	if rb.Count() != 1 {
		t.Errorf("Count() after 1 add = %d, want 1", rb.Count())
	}

	rb.Add("cmd2")
	rb.Add("cmd3")
	if rb.Count() != 3 {
		t.Errorf("Count() after 3 adds = %d, want 3", rb.Count())
	}

	rb.Drain()
	if rb.Count() != 0 {
		t.Errorf("Count() after Drain() = %d, want 0", rb.Count())
	}
}

func TestRingBuffer_ConcurrentAdd(t *testing.T) {
	rb := New(100, func(s string) string { return s })

	var wg sync.WaitGroup
	numGoroutines := 10
	itemsPerGoroutine := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				rb.Add(fmt.Sprintf("g%d-e%d", goroutineID, j))
			}
		}(i)
	}

	wg.Wait()

	totalUnique := numGoroutines * itemsPerGoroutine
	expectedCount := rb.capacity
	if totalUnique < rb.capacity {
		expectedCount = totalUnique
	}

	if rb.Count() != expectedCount {
		t.Errorf("Count() after concurrent adds = %d, want %d", rb.Count(), expectedCount)
	}

	items := rb.Drain()
	if len(items) != expectedCount {
		t.Errorf("Drain() returned %d items, want %d", len(items), expectedCount)
	}
}

func TestRingBuffer_ConcurrentAddAndDrain(t *testing.T) {
	rb := New(50, func(s string) string { return s })

	var wg sync.WaitGroup
	numWriters := 5
	itemsPerWriter := 20

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for j := 0; j < itemsPerWriter; j++ {
				rb.Add(fmt.Sprintf("w%d-e%d", writerID, j))
			}
		}(i)
	}

	drainResults := make(chan int, 10)
	wg.Add(1)
	go func() {
		defer wg.Done()
		totalDrained := 0
		for i := 0; i < 5; i++ {
			items := rb.Drain()
			totalDrained += len(items)
		}
		drainResults <- totalDrained
	}()

	wg.Wait()
	close(drainResults)

	remaining := rb.Drain()

	if rb.Count() != 0 {
		t.Errorf("Count() after all operations = %d, want 0", rb.Count())
	}

	rb.Add("after")
	if rb.Count() != 1 {
		t.Errorf("Count() after post-concurrent add = %d, want 1", rb.Count())
	}

	_ = remaining
}

func TestRingBuffer_HeadWrapAround(t *testing.T) {
	rb := New(3, func(s string) string { return s })

	for i := 0; i < 7; i++ {
		rb.Add(string(rune('a' + i)))
	}

	if rb.head != 1 {
		t.Errorf("head = %d, want 1 after 7 adds in capacity 3 buffer", rb.head)
	}

	items := rb.Drain()
	expected := []string{"e", "f", "g"}
	for i, exp := range expected {
		if items[i] != exp {
			t.Errorf("items[%d] = %q, want %q", i, items[i], exp)
		}
	}
}

func TestRingBuffer_Capacity(t *testing.T) {
	rb := New(42, func(s string) string { return s })
	if rb.Capacity() != 42 {
		t.Errorf("Capacity() = %d, want 42", rb.Capacity())
	}
}

func TestRingBuffer_CustomKeyFunc(t *testing.T) {
	// use a key function that groups by prefix
	type item struct {
		category string
		value    string
	}
	rb := New(10, func(i item) string { return i.category })

	rb.Add(item{category: "cmd", value: "first"})
	rb.Add(item{category: "cmd", value: "second"}) // same key, should be deduplicated

	if rb.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (same key should deduplicate)", rb.Count())
	}

	rb.Add(item{category: "flag", value: "third"}) // different key
	if rb.Count() != 2 {
		t.Errorf("Count() = %d, want 2 (different key should be distinct)", rb.Count())
	}
}


func TestRingBuffer_Refill_Basic(t *testing.T) {
	rb := New(5, func(s string) string { return s })

	rb.Add("cmd1")
	rb.Add("cmd2")
	rb.Add("cmd3")

	drained := rb.Drain()
	if len(drained) != 3 {
		t.Fatalf("Drain() returned %d items, want 3", len(drained))
	}
	if rb.Count() != 0 {
		t.Fatalf("Count() after Drain() = %d, want 0", rb.Count())
	}

	rb.Refill(drained)
	if rb.Count() != 3 {
		t.Errorf("Count() after Refill() = %d, want 3", rb.Count())
	}

	result := rb.Drain()
	if len(result) != 3 {
		t.Fatalf("Drain() after Refill() returned %d items, want 3", len(result))
	}
	for i, want := range []string{"cmd1", "cmd2", "cmd3"} {
		if result[i] != want {
			t.Errorf("result[%d] = %q, want %q", i, result[i], want)
		}
	}
}

func TestRingBuffer_Refill_WithNewEvents(t *testing.T) {
	rb := New(10, func(s string) string { return s })

	rb.Add("old1")
	rb.Add("old2")

	drained := rb.Drain()

	// new events arrive while submitting
	rb.Add("new1")
	rb.Add("new2")

	// submit failed - refill old events
	rb.Refill(drained)

	if rb.Count() != 4 {
		t.Errorf("Count() = %d, want 4 (2 new + 2 refilled)", rb.Count())
	}

	result := rb.Drain()
	if len(result) != 4 {
		t.Fatalf("Drain() returned %d items, want 4", len(result))
	}
	// new events were added first, then old events refilled after
	expected := []string{"new1", "new2", "old1", "old2"}
	for i, want := range expected {
		if result[i] != want {
			t.Errorf("result[%d] = %q, want %q", i, result[i], want)
		}
	}
}

func TestRingBuffer_Refill_OverflowEvictsOldest(t *testing.T) {
	rb := New(3, func(s string) string { return s })

	rb.Add("old1")
	rb.Add("old2")
	rb.Add("old3")

	drained := rb.Drain()

	// buffer fills with new events while submitting
	rb.Add("new1")
	rb.Add("new2")
	rb.Add("new3")

	// submit failed - refill, but buffer is full so oldest (new events) get evicted
	rb.Refill(drained)

	if rb.Count() != 3 {
		t.Errorf("Count() = %d, want 3 (capacity)", rb.Count())
	}

	result := rb.Drain()
	// old events overwrite new events (FIFO eviction)
	expected := []string{"old1", "old2", "old3"}
	for i, want := range expected {
		if result[i] != want {
			t.Errorf("result[%d] = %q, want %q", i, result[i], want)
		}
	}
}

func TestRingBuffer_Refill_DedupRespected(t *testing.T) {
	rb := New(10, func(s string) string { return s })

	rb.Add("cmd1")
	rb.Add("cmd2")

	drained := rb.Drain()

	// same key arrives as a new event while submitting
	rb.Add("cmd1")

	// refill - cmd1 should be deduped since it is already in the buffer
	rb.Refill(drained)

	if rb.Count() != 2 {
		t.Errorf("Count() = %d, want 2 (cmd1 deduped, cmd2 added)", rb.Count())
	}

	result := rb.Drain()
	if len(result) != 2 {
		t.Fatalf("Drain() returned %d items, want 2", len(result))
	}
	// cmd1 was already in buffer (from new Add), cmd2 added by Refill
	if result[0] != "cmd1" {
		t.Errorf("result[0] = %q, want %q", result[0], "cmd1")
	}
	if result[1] != "cmd2" {
		t.Errorf("result[1] = %q, want %q", result[1], "cmd2")
	}
}
