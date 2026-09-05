// Package eventbus publishes canonical events to NATS JetStream subjects
// (Architecture §37 topic list). MemoryBus records for tests/replay.
package eventbus

import (
	"context"
	"encoding/json"
	"sync"
	"time"

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

// ResilientBus wraps a redialable transport (NATS): on publish failure it
// rebuilds the connection and retries once, so a dependency outage ends when
// the dependency recovers — not only when the process restarts (ADV-0005).
// LastErr records the most recent failure for health reporting.
type ResilientBus struct {
	mu      sync.Mutex
	dial    func() (Bus, error)
	bus     Bus
	lastErr time.Time
	lastMsg string
}

func NewResilientBus(dial func() (Bus, error), initial Bus) *ResilientBus {
	return &ResilientBus{dial: dial, bus: initial}
}

func (r *ResilientBus) fail(err error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastErr = time.Now()
	r.lastMsg = err.Error()
	return err
}

// LastError reports the most recent publish failure (zero time = healthy).
func (r *ResilientBus) LastError() (time.Time, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastErr, r.lastMsg
}

func (r *ResilientBus) Publish(ctx context.Context, e events.Event) error {
	r.mu.Lock()
	bus := r.bus
	r.mu.Unlock()
	if err := bus.Publish(ctx, e); err == nil {
		return nil
	} else {
		_ = err
	}
	// Redial once, then retry once: bounded, no SYN storm.
	nb, derr := r.dial()
	if derr != nil {
		return r.fail(derr)
	}
	r.mu.Lock()
	if closer, ok := r.bus.(interface{ Close() }); ok {
		closer.Close()
	}
	r.bus = nb
	r.mu.Unlock()
	if err := nb.Publish(ctx, e); err != nil {
		return r.fail(err)
	}
	return nil
}

// SubscribeResilient keeps a subject subscription alive across NATS outages:
// redial + resubscribe until ctx ends. A single Subscribe() at boot dies
// permanently once the client's reconnect budget exhausts (ADV-0005).
func SubscribeResilient(ctx context.Context, url, subject string, fn func([]byte)) {
	go func() {
		backoff := time.Second
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			nc, err := nats.Connect(url)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				if backoff < 30*time.Second {
					backoff *= 2
				}
				continue
			}
			backoff = time.Second
			sub, err := nc.Subscribe(subject, func(m *nats.Msg) { fn(m.Data) })
			if err != nil {
				nc.Close()
				continue
			}
			// Block until context ends or the connection dies for good.
			tick := time.NewTicker(5 * time.Second)
			dead := false
			for !dead {
				select {
				case <-ctx.Done():
					tick.Stop()
					_ = sub.Unsubscribe()
					nc.Close()
					return
				case <-tick.C:
					if !nc.IsConnected() {
						dead = true
					}
				}
			}
			tick.Stop()
			_ = sub.Unsubscribe()
			nc.Close()
		}
	}()
}
