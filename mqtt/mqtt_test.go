package mqtt

import (
	"errors"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// mockClient satisfies the mqtt.Client interface
// We only implement the methods the Actor actually calls.
type mockClient struct {
	mqtt.Client // Embed to satisfy the interface automatically
	connected   bool
	published   chan string
}

func (m *mockClient) Connect() mqtt.Token {
	m.connected = true
	return &mockToken{}
}

func (m *mockClient) IsConnected() bool {
	return m.connected
}

func (m *mockClient) Subscribe(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token {
	return &mockToken{}
}

func (m *mockClient) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	m.published <- topic
	return &mockToken{}
}

func (m *mockClient) Disconnect(quiesce uint) {
	m.connected = false
}

// mockToken satisfies the mqtt.Token interface
type mockToken struct {
	mqtt.Token
	err error
}

func (m *mockToken) Wait() bool                     { return true }
func (m *mockToken) WaitTimeout(time.Duration) bool { return true }
func (m *mockToken) Error() error                   { return m.err }

func TestActor_PublishAndLifecycle(t *testing.T) {
	// 1. Setup
	mock := &mockClient{published: make(chan string, 1)}
	opts := mqtt.NewClientOptions()
	actor := NewActor("test-actor", opts)

	// Since we are in the same package, we can set the client directly
	// for the Publish test, or simulate Run.
	actor.client = mock
	actor.mu.Lock() // Initialize the client pointer
	actor.client = mock
	actor.mu.Unlock()

	// 2. Test Publish
	testTopic := "cmd/test"
	go func() {
		err := actor.Publish(testTopic, 1, false, "payload")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	}()

	select {
	case topic := <-mock.published:
		if topic != testTopic {
			t.Errorf("expected topic %s, got %s", testTopic, topic)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("publish timed out")
	}
}

func TestActor_ConnectionLossSignal(t *testing.T) {
	errChan := make(chan error, 1)
	opts := mqtt.NewClientOptions()

	// 1. Setup the handler exactly as you do in your Actor.Run
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		select {
		case errChan <- err:
		default:
		}
	})

	// 2. Trigger the field directly.
	// The field name is OnConnectionLost (with the 'ion')
	expectedErr := errors.New("network cable pulled")
	if opts.OnConnectionLost != nil {
		opts.OnConnectionLost(nil, expectedErr)
	}

	// 3. Verify
	select {
	case err := <-errChan:
		if err != expectedErr {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("connection lost signal not caught")
	}
}
