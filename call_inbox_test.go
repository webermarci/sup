package sup_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/webermarci/sup"
)

func TestCallInbox(t *testing.T) {
	inbox := sup.NewCallInbox[string, int]()
	go func() {
		req := <-inbox.Receive()
		req.Reply(len(req.Payload()), nil)
	}()

	got, err := inbox.Call[int](t.Context(), "calculate_length")
	if err != nil {
		t.Fatal(err)
	}
	if got != 16 {
		t.Fatalf("expected 16, got %d", got)
	}
}

func TestCallInboxReplyError(t *testing.T) {
	inbox := sup.NewCallInbox[string, string]()
	expectedErr := errors.New("hardware failure")
	go func() {
		(<-inbox.Receive()).Reply("", expectedErr)
	}()

	if _, err := inbox.Call[string](t.Context(), "ping"); !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
}

func TestCallInboxSupportsHeterogeneousResponses(t *testing.T) {
	inbox := sup.NewCallInbox[testCallMessage, testCallResponse]()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 2 {
			request := <-inbox.Receive()
			switch message := request.Payload().(type) {
			case getTestStatus:
				request.Reply(testStatus{running: true}, nil)
			case getTestConfig:
				request.Reply(testConfig{speed: message.defaultSpeed}, nil)
			}
		}
	}()

	status, err := inbox.Call[testStatus](t.Context(), getTestStatus{})
	if err != nil {
		t.Fatal(err)
	}
	if !status.running {
		t.Fatal("expected running status")
	}

	config, err := inbox.Call[testConfig](t.Context(), getTestConfig{defaultSpeed: 42})
	if err != nil {
		t.Fatal(err)
	}
	if config.speed != 42 {
		t.Fatalf("expected speed 42, got %d", config.speed)
	}
	<-done
}

func TestCallInboxRejectsUnexpectedResponseType(t *testing.T) {
	inbox := sup.NewCallInbox[testCallMessage, testCallResponse]()
	go func() {
		request := <-inbox.Receive()
		if !request.Reply(testConfig{speed: 42}, nil) {
			t.Error("expected mismatched response to reach the caller")
		}
	}()

	_, err := inbox.Call[testStatus](t.Context(), getTestStatus{})
	if !errors.Is(err, sup.ErrUnexpectedResponseType) {
		t.Fatalf("expected unexpected response type, got %v", err)
	}
}

func TestCallInboxReplyErrorDoesNotRequireExpectedResponseType(t *testing.T) {
	expectedErr := errors.New("request failed")
	inbox := sup.NewCallInbox[testCallMessage, testCallResponse]()
	go func() {
		request := <-inbox.Receive()
		request.Reply(nil, expectedErr)
	}()

	_, err := inbox.Call[testStatus](t.Context(), getTestStatus{})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected reply error, got %v", err)
	}
}

func TestCallInboxAllowsNilInterfaceResponse(t *testing.T) {
	inbox := sup.NewCallInbox[string, any]()
	go func() {
		request := <-inbox.Receive()
		request.Reply(nil, nil)
	}()

	response, err := inbox.Call[any](t.Context(), "request")
	if err != nil {
		t.Fatal(err)
	}
	if response != nil {
		t.Fatalf("expected nil response, got %v", response)
	}
}

func TestCallInboxWaitingForReplyCanBeCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	inbox := sup.NewCallInbox[string, int]()
	queued := make(chan struct{})
	go func() {
		<-inbox.Receive()
		close(queued)
	}()
	go func() {
		<-queued
		cancel()
	}()

	if _, err := inbox.Call[int](ctx, "no reply"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestCallRequestExposesCallerContext(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(t.Context(), key{}, "value")
	inbox := sup.NewCallInbox[string, string]()

	done := make(chan struct{})
	go func() {
		defer close(done)
		req := <-inbox.Receive()
		if got := req.Context().Value(key{}); got != "value" {
			t.Errorf("expected caller context value, got %v", got)
		}
		req.Reply("ok", nil)
	}()

	if got, err := inbox.Call[string](ctx, "request"); err != nil || got != "ok" {
		t.Fatalf("expected reply ok, got %q, %v", got, err)
	}
	<-done
}

func TestCallRequestReplyOnlySucceedsOnce(t *testing.T) {
	inbox := sup.NewCallInbox[string, string]()
	received := make(chan struct{})
	go func() {
		req := <-inbox.Receive()
		if !req.Reply("first", nil) {
			t.Error("expected first reply to succeed")
		}
		if req.Reply("second", nil) {
			t.Error("expected second reply to fail")
		}
		close(received)
	}()

	if got, err := inbox.Call[string](t.Context(), "request"); err != nil || got != "first" {
		t.Fatalf("expected first reply, got %q, %v", got, err)
	}
	<-received
}

func TestCallRequestReplyHasOneConcurrentWinner(t *testing.T) {
	inbox := sup.NewCallInbox[string, int]()
	requestCh := make(chan sup.CallRequest[string, int], 1)
	go func() { requestCh <- <-inbox.Receive() }()

	callDone := make(chan error, 1)
	go func() {
		_, err := inbox.Call[int](t.Context(), "request")
		callDone <- err
	}()

	request := <-requestCh
	const replies = 32
	results := make(chan bool, replies)
	var group sync.WaitGroup
	group.Add(replies)
	for i := range replies {
		go func(value int) {
			defer group.Done()
			results <- request.Reply(value, nil)
		}(i)
	}
	group.Wait()
	close(results)

	winners := 0
	for succeeded := range results {
		if succeeded {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("expected exactly one successful reply, got %d", winners)
	}
	if err := <-callDone; err != nil {
		t.Fatalf("call failed after concurrent replies: %v", err)
	}
}

func TestCallRequestReplyFailsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	inbox := sup.NewCallInbox[string, string]()
	reqCh := make(chan sup.CallRequest[string, string], 1)
	go func() { reqCh <- <-inbox.Receive() }()

	done := make(chan error, 1)
	go func() {
		_, err := inbox.Call[string](ctx, "request")
		done <- err
	}()
	req := <-reqCh
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if req.Reply("late", nil) {
		t.Fatal("expected late reply to fail")
	}
}

func TestCallInboxCallCanBeCanceledWhileAdmissionIsBlocked(t *testing.T) {
	inbox := sup.NewCallInbox[string, int]()
	baseCtx, cancel := context.WithCancel(t.Context())
	ctx := &doneObservedContext{
		Context:  baseCtx,
		observed: make(chan struct{}),
	}
	done := make(chan error, 1)
	go func() {
		_, err := inbox.Call[int](ctx, "blocked")
		done <- err
	}()

	// The context wrapper reports when the inbox send has evaluated its
	// cancellation channel. With no receiver, admission is now blocked.
	<-ctx.observed
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked call did not stop after cancellation")
	}
}

type doneObservedContext struct {
	context.Context
	observed chan struct{}
	sync.Once
}

type testCallMessage interface {
	testCallMessage()
}

type getTestStatus struct{}

func (getTestStatus) testCallMessage() {}

type getTestConfig struct {
	defaultSpeed int
}

func (getTestConfig) testCallMessage() {}

type testCallResponse interface {
	testCallResponse()
}

type testStatus struct {
	running bool
}

func (testStatus) testCallResponse() {}

type testConfig struct {
	speed int
}

func (testConfig) testCallResponse() {}

func (c *doneObservedContext) Done() <-chan struct{} {
	c.Once.Do(func() { close(c.observed) })
	return c.Context.Done()
}
