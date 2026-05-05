package sup

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRouter_RoundRobin(t *testing.T) {
	var calls [3]int

	r := NewRouter(RoundRobin,
		func() { calls[0]++ },
		func() { calls[1]++ },
		func() { calls[2]++ },
	)

	for range 4 {
		r.Next()()
	}

	if calls[0] != 2 || calls[1] != 1 || calls[2] != 1 {
		t.Errorf("unexpected distribution: %v", calls)
	}
}

func TestRouter_Random(t *testing.T) {
	r := NewRouter(Random, func() {}, func() {})

	for range 100 {
		fn := r.Next()
		if fn == nil {
			t.Fatal("Next() returned nil")
		}
	}
}

func TestRouter_Sticky(t *testing.T) {
	r := NewRouter(RoundRobin, 1, 2, 3)

	if val := r.Sticky(0); val != 1 {
		t.Errorf("expected 1, got %d", val)
	}

	if val := r.Sticky(5); val != 3 {
		t.Errorf("expected 3, got %d", val)
	}
}

func TestRouter_Broadcast(t *testing.T) {
	var sum int64
	r := NewRouter(RoundRobin, 10, 20, 30)

	r.Broadcast(func(val int) {
		atomic.AddInt64(&sum, int64(val))
	})

	if sum != 60 {
		t.Errorf("expected sum 60, got %d", sum)
	}
}

func TestRouter_FanOutWait(t *testing.T) {
	var sum int64
	r := NewRouter(RoundRobin, 1, 1, 1, 1, 1)

	r.FanOutWait(func(val int) {
		atomic.AddInt64(&sum, int64(val))
	})

	if sum != 5 {
		t.Errorf("expected sum 5, got %d", sum)
	}
}

func TestRouter_Retry(t *testing.T) {
	errFail := errors.New("fail")
	var attempts int

	r := NewRouter(RoundRobin, func() error {
		attempts++
		if attempts < 3 {
			return errFail
		}
		return nil
	})

	err := r.Retry(2, func(f func() error) error { return f() })
	if !errors.Is(err, errFail) {
		t.Errorf("expected errFail, got %v", err)
	}

	attempts = 0

	err = r.Retry(3, func(f func() error) error { return f() })
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestRouter_Concurrency(t *testing.T) {
	r := NewRouter(RoundRobin, 1, 2)
	const goroutines = 100
	const increments = 1000
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range increments {
				r.Next()
			}
		}()
	}
	wg.Wait()
}

func TestRouter_EmptyPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic for empty routees")
		}
	}()
	NewRouter[func()](RoundRobin)
}
