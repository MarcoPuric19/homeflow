package integrations

import "context"

// Connector is the provider-neutral boundary for optional external systems.
// Implementations must be disabled unless explicitly configured.
type Connector interface {
	Name() string
	Health(ctx context.Context) error
	Publish(ctx context.Context, event Event) error
}

type Event struct {
	ID      string
	Name    string
	Version int
	Payload []byte
}
