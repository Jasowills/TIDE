// Package pipeline is THE production processing path (Architecture §2.8).
// Every entrypoint (HTTP, MQTT, adapters, FleetSim) and replay funnels
// through Pipeline.Process. A parallel "replay engine" duplicating this
// logic is an automatic design rejection — there is only this function.
package pipeline

import (
	"context"
	"sync"

	"github.com/tide-telematics/tide/internal/dedup"
	"github.com/tide-telematics/tide/internal/detectors"
	"github.com/tide-telematics/tide/internal/eventbus"
	"github.com/tide-telematics/tide/internal/state"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
	"github.com/tide-telematics/tide/schemas/events"
)

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

// Process ingests one canonical point end-to-end.
func (p *Pipeline) Process(ctx context.Context, t ctelemetry.Telemetry) ([]events.Event, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	dup, err := p.Dedup.Seen(ctx, t.DedupKey())
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, nil // idempotent: duplicates change nothing
	}
	if err := p.Log.Append(ctx, t); err != nil {
		return nil, err
	}
	prev, _, err := p.States.Get(ctx, t.VehicleID)
	if err != nil {
		return nil, err
	}
	var eng state.Engine
	next, _ := eng.Apply(prev, t)
	if err := p.States.Set(ctx, next); err != nil {
		return nil, err
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
			return evs, err
		}
	}
	return evs, nil
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
