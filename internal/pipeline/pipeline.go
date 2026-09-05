// Package pipeline is THE production processing path (Architecture §2.8).
// Every entrypoint (HTTP, MQTT, adapters, FleetSim) and replay funnels
// through Pipeline.Process. A parallel "replay engine" duplicating this
// logic is an automatic design rejection — there is only this function.
package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tide-telematics/tide/internal/dedup"
	"github.com/tide-telematics/tide/internal/detectors"
	"github.com/tide-telematics/tide/internal/eventbus"
	"github.com/tide-telematics/tide/internal/state"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
	"github.com/tide-telematics/tide/schemas/events"
)

// MaxFutureSkew bounds how far ahead of wall-clock a point's event time may
// be. Beyond it the point is rejected, never silently absorbed (ADV-0002).
const MaxFutureSkew = time.Hour

// TelemetryLog is durable ingestion storage (Postgres in prod).
type TelemetryLog interface {
	Append(ctx context.Context, t ctelemetry.Telemetry) error
}

type MemoryLog struct {
	mu sync.Mutex
	All []ctelemetry.Telemetry
}

func (m *MemoryLog) Append(_ context.Context, t ctelemetry.Telemetry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.All = append(m.All, t)
	return nil
}

// Rules evaluates after detection (Phase 6 wires the real engine; nil = off).
type Rules interface {
	Evaluate(ctx context.Context, t ctelemetry.Telemetry, s state.VehicleState, evs []events.Event) []events.Event
}

type Pipeline struct {
	Dedup     dedup.Store
	States    state.Store
	Log       TelemetryLog
	Bus       eventbus.Bus
	Detectors *detectors.Tracker
	Geo       GeoEvaluator
	Rules     Rules
}

// GeoEvaluator is *geo.Tracker in prod; nil disables geofencing.
type GeoEvaluator interface {
	Evaluate(ctx context.Context, t ctelemetry.Telemetry) []events.Event
}

// PublishError carries the already-computed events when bus delivery fails,
// so callers can persist them durably instead of stranding them (ADV-0005).
// It never indicates the point was unprocessed: dedup, log and state all ran.
type PublishError struct {
	Events []events.Event
	Err    error
}

func (e *PublishError) Error() string { return fmt.Sprintf("publish: %v", e.Err) }
func (e *PublishError) Unwrap() error { return e.Err }

// Process ingests one canonical point end-to-end. The dup flag reports an
// absorbed duplicate (idempotent no-op); callers must not duplicate durable
// side effects (e.g. PG appends) for dup points.
func (p *Pipeline) Process(ctx context.Context, t ctelemetry.Telemetry) ([]events.Event, bool, error) {
	if err := t.Validate(); err != nil {
		return nil, false, err
	}
	// Event-time sanity: points absurdly far in the future would pin the
	// watermark (LastSeen) and freeze this vehicle's state forever (ADV-0002).
	// Tolerance is one hour for device clock drift; replay of recorded
	// windows is unaffected (historical timestamps predate now).
	if t.Timestamp.After(time.Now().Add(MaxFutureSkew)) {
		return nil, false, fmt.Errorf("timestamp %s too far in future", t.Timestamp.Format(time.RFC3339))
	}
	dup, err := p.Dedup.Seen(ctx, t.DedupKey())
	if err != nil {
		return nil, false, err
	}
	if dup {
		return nil, true, nil // idempotent: duplicates change nothing
	}
	if err := p.Log.Append(ctx, t); err != nil {
		return nil, false, err
	}
	prev, _, err := p.States.Get(ctx, t.VehicleID)
	if err != nil {
		return nil, false, err
	}
	var eng state.Engine
	next, _ := eng.Apply(prev, t)
	if err := p.States.Set(ctx, next); err != nil {
		return nil, false, err
	}
	evs := p.Detectors.Detect(t, next)
	if p.Geo != nil {
		evs = append(evs, p.Geo.Evaluate(ctx, t)...)
	}
	if p.Rules != nil {
		evs = append(evs, p.Rules.Evaluate(ctx, t, next, evs)...)
	}
	for _, e := range evs {
		if err := p.Bus.Publish(ctx, e); err != nil {
			return evs, false, &PublishError{Events: evs, Err: err}
		}
	}
	return evs, false, nil
}

// NewTestPipeline wires memory implementations for tests/replay/FleetSim.
func NewTestPipeline() (*Pipeline, *eventbus.MemoryBus, *MemoryLog) {
	bus := &eventbus.MemoryBus{}
	log := &MemoryLog{}
	return &Pipeline{
		Dedup:     dedup.NewMemoryStore(0),
		States:    state.NewMemoryStore(),
		Log:       log,
		Bus:       bus,
		Detectors: detectors.NewTracker(detectors.DefaultConfig()),
	}, bus, log
}
