package throttle

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestThrottle_New(t *testing.T) {
	th := New(5 * time.Second)
	if th == nil {
		t.Fatal("New returned nil")
	}
	if th.cooldown != 5*time.Second {
		t.Errorf("cooldown = %v, want 5s", th.cooldown)
	}
}

func TestThrottle_TryFlush(t *testing.T) {
	tests := []struct {
		name     string
		cooldown time.Duration
		setup    func(*Throttle)
		want     bool
	}{
		{
			name:     "first call returns true",
			cooldown: time.Hour,
			want:     true,
		},
		{
			name:     "second call within cooldown returns false",
			cooldown: time.Hour,
			setup: func(th *Throttle) {
				th.TryFlush() // claim the slot
			},
			want: false,
		},
		{
			name:     "call after cooldown returns true",
			cooldown: 1 * time.Millisecond,
			setup: func(th *Throttle) {
				th.TryFlush()
				time.Sleep(5 * time.Millisecond) // wait past cooldown
			},
			want: true,
		},
		{
			name:     "zero cooldown always allows",
			cooldown: 0,
			setup: func(th *Throttle) {
				th.TryFlush()
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th := New(tt.cooldown)
			if tt.setup != nil {
				tt.setup(th)
			}
			got := th.TryFlush()
			if got != tt.want {
				t.Errorf("TryFlush() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestThrottle_RecordFlush(t *testing.T) {
	th := New(time.Hour)

	// first TryFlush succeeds
	if !th.TryFlush() {
		t.Fatal("first TryFlush should succeed")
	}

	// manually record a flush -- should update lastFlush to now,
	// so a subsequent TryFlush within the hour cooldown should fail
	th.RecordFlush()

	if th.TryFlush() {
		t.Error("TryFlush after RecordFlush within cooldown should return false")
	}
}

func TestThrottle_RecordFlush_ResetsCooldownWindow(t *testing.T) {
	th := New(10 * time.Millisecond)

	// claim slot
	th.TryFlush()
	time.Sleep(15 * time.Millisecond)

	// record a flush, resetting the window
	th.RecordFlush()

	// now TryFlush should fail because RecordFlush just updated lastFlush
	if th.TryFlush() {
		t.Error("TryFlush immediately after RecordFlush should return false")
	}

	// wait for cooldown to expire
	time.Sleep(15 * time.Millisecond)
	if !th.TryFlush() {
		t.Error("TryFlush after cooldown elapsed should return true")
	}
}

func TestThrottle_ConcurrentAccess(t *testing.T) {
	th := New(1 * time.Millisecond)

	var wg sync.WaitGroup
	var trueCount atomic.Int64

	// many goroutines racing to TryFlush
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if th.TryFlush() {
				trueCount.Add(1)
			}
		}()
	}
	wg.Wait()

	// at least one should succeed, and the count should be reasonable
	// (not 100, because of the cooldown)
	got := trueCount.Load()
	if got < 1 {
		t.Error("at least one goroutine should have succeeded")
	}

	// concurrent RecordFlush shouldn't panic
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			th.RecordFlush()
		}()
	}
	wg.Wait()
}
