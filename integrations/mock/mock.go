package mock

import (
	"context"
	"sync"

	"homeflow/integrations"
)

type Connector struct {
	mu     sync.Mutex
	events []integrations.Event
}

func (c *Connector) Name() string                 { return "mock" }
func (c *Connector) Health(context.Context) error { return nil }
func (c *Connector) Publish(_ context.Context, event integrations.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
	return nil
}

func (c *Connector) Events() []integrations.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]integrations.Event(nil), c.events...)
}
