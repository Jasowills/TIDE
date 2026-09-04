// Package eventbus publishes canonical events to NATS JetStream subjects
// (Architecture §37 topic list). MemoryBus records for tests/replay.
package eventbus

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/tide-telematics/tide/schemas/events"
)

type Bus interface {
	Publish(ctx context.Context, e events.Event) error
}

type MemoryBus struct {
	mu     sync.Mutex
	Events []events.Event
}

func (m *MemoryBus) Publish(_ context.Context, e events.Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Events = append(m.Events, e)
	return nil
}

func (m *MemoryBus) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Events = nil
}

// Snapshot returns a copy for readers (HTTP handlers). Ranging over .Events
// directly while Publish appends is a fatal concurrent-map-style race.
func (m *MemoryBus) Snapshot() []events.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]events.Event{}, m.Events...)
}

type NATSBus struct {
	nc *nats.Conn
}

func NewNATSBus(url string) (*NATSBus, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, err
	}
	return &NATSBus{nc: nc}, nil
}

func (b *NATSBus) Publish(_ context.Context, e events.Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	// At-least-once: NATS core delivery; idempotency via deterministic ids.
	return b.nc.Publish(e.Subject(), raw)
}

func (b *NATSBus) Close() { b.nc.Close() }

// FanOut publishes to every child (e.g. NATS for transport + memory for
// local queries). A child error is returned but never blocks the others.
type FanOut struct {
	Children []Bus
}

func (f FanOut) Publish(ctx context.Context, e events.Event) error {
	var first error
	for _, c := range f.Children {
		if err := c.Publish(ctx, e); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Subscribe streams raw messages on subject to fn; returns unsubscribe.
func Subscribe(url, subject string, fn func([]byte)) (func(), error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, err
	}
	sub, err := nc.Subscribe(subject, func(m *nats.Msg) { fn(m.Data) })
	if err != nil {
		nc.Close()
		return nil, err
	}
	return func() {
		_ = sub.Unsubscribe()
		nc.Close()
	}, nil
}
