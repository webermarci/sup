package mesh

import (
	"context"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/webermarci/sup"
)

// Client is an actor responsible for managing a connection to a NATS server.
type Client struct {
	*sup.BaseActor
	opts nats.Options
	conn *nats.Conn
	mu   sync.RWMutex
}

// NewClient creates a new Client actor with the given name and NATS connection options.
func NewClient(name string, opts nats.Options) *Client {
	return &Client{
		BaseActor: sup.NewBaseActor(name),
		opts:      opts,
	}
}

// Conn returns the current NATS connection managed by the Client actor.
// It is safe for concurrent use.
func (c *Client) Conn() *nats.Conn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

// Run starts the Client actor, establishing a connection to the NATS server and
// keeping it alive until the context is canceled. It handles connection errors and
// ensures proper cleanup of resources when stopping.
func (c *Client) Run(ctx context.Context) error {
	nc, err := c.opts.Connect()
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = nc
	c.mu.Unlock()

	<-ctx.Done()

	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.mu.Unlock()
	return nil
}
