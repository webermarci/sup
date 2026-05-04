package hub

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRoundRobin(t *testing.T) {
	var calls [3]int

	h := NewRoundRobin(
		func() { calls[0]++ },
		func() { calls[1]++ },
		func() { calls[2]++ },
	)

	// Execute 4 times: should go 0, 1, 2, 0
	for range 4 {
		h.Next()()
	}

	if calls[0] != 2 || calls[1] != 1 || calls[2] != 1 {
		t.Errorf("unexpected distribution: %v", calls)
	}
}

func TestRandom(t *testing.T) {
	h := NewRandom(func() {}, func() {})

	for range 100 {
		fn := h.Next()
		if fn == nil {
			t.Fatal("Next() returned nil")
		}
	}
}

func TestSticky(t *testing.T) {
	h := NewRoundRobin(1, 2, 3)

	if val := h.Sticky(0); val != 1 {
		t.Errorf("expected 1, got %d", val)
	}
	if val := h.Sticky(5); val != 3 { // 5 % 3 = 2 -> routees[2] = 3
		t.Errorf("expected 3, got %d", val)
	}
}

func TestBroadcast(t *testing.T) {
	var sum int64
	h := NewRoundRobin(10, 20, 30)

	h.Broadcast(func(val int) {
		atomic.AddInt64(&sum, int64(val))
	})

	if sum != 60 {
		t.Errorf("expected sum 60, got %d", sum)
	}
}

func TestFanOutWait(t *testing.T) {
	var sum int64
	h := NewRoundRobin(1, 1, 1, 1, 1)

	h.FanOutWait(func(val int) {
		atomic.AddInt64(&sum, int64(val))
	})

	if sum != 5 {
		t.Errorf("expected sum 5, got %d", sum)
	}
}

func TestRetry(t *testing.T) {
	errFail := errors.New("fail")
	var attempts int

	h := NewRoundRobin(func() error {
		attempts++
		if attempts < 3 {
			return errFail
		}
		return nil
	})

	err := h.Retry(2, func(f func() error) error { return f() })
	if !errors.Is(err, errFail) {
		t.Errorf("expected errFail, got %v", err)
	}

	attempts = 0
	err = h.Retry(3, func(f func() error) error { return f() })
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestConcurrency(t *testing.T) {
	h := NewRoundRobin(1, 2)
	const goroutines = 100
	const increments = 1000
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range increments {
				h.Next()
			}
		}()
	}
	wg.Wait()
}

func TestEmptyPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic for empty routees")
		}
	}()
	NewRoundRobin[func()]()
}
