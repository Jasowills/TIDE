// Package registry wires env-configured upstream sources to the processing
// pipeline. Provider names live HERE, inside /adapters/**, where the
// provider-isolation lint permits them — engine code never branches on
// provider identity. An unconfigured adapter simply doesn't exist: nothing
// is fabricated, and heartbeats report each adapter's ACTUAL Health().
package registry

import (
	"context"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/tide-telematics/tide/adapters"
	traccaradapter "github.com/tide-telematics/tide/adapters/traccar"
	mqttadapter "github.com/tide-telematics/tide/adapters/mqtt"
	"github.com/tide-telematics/tide/internal/pipeline"
	"github.com/tide-telematics/tide/schemas/events"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

type Adapter struct {
	name    string
	src     adapters.TelemetrySource
	emit    func(ctx context.Context, t ctelemetry.Telemetry) error
	count   atomic.Int64
	started time.Time
}

// Name is the adapter's registry identity (connections screen, heartbeats).
func (r *Adapter) Name() string { return r.name }

// Source exposes the underlying TelemetrySource (health, capabilities).
func (r *Adapter) Source() adapters.TelemetrySource { return r.src }

func defaultTenant() string {
	if v := os.Getenv("TIDE_TENANT"); v != "" {
		return v
	}
	return "default"
}

func emitting(p *pipeline.Pipeline, count *atomic.Int64) func(ctx context.Context, t ctelemetry.Telemetry) error {
	return func(ctx context.Context, t ctelemetry.Telemetry) error {
		_, _, err := p.Process(ctx, t)
		if err != nil {
			return err
		}
		count.Add(1)
		return nil
	}
}

// Configure builds every upstream with credentials present.
// flespi is intentionally absent: its live transport is still a stub
// (Subscribe blocks without subscribing) — advertising it HEALTHY would be
// dishonest. It ships as normalization + contract tests until the live path
// lands.
func Configure(p *pipeline.Pipeline) []*Adapter {
	var out []*Adapter
	if broker := os.Getenv("TIDE_MQTT_BROKER"); broker != "" {
		mapPath := os.Getenv("TIDE_MQTT_MAPPING")
		if mapPath == "" {
			mapPath = "examples/mqtt-mapping.yaml"
		}
		raw, err := os.ReadFile(mapPath)
		if err != nil {
			log.Printf("adapters: mqtt mapping missing (%v) — subscriber off", err)
		} else if mc, err := mqttadapter.LoadMapping(raw); err != nil {
			log.Printf("adapters: bad mqtt mapping: %v", err)
		} else {
			mc.Broker = broker
			r := &Adapter{name: "mqtt", started: time.Now()}
			r.emit = emitting(p, &r.count)
			r.src = mqttadapter.New(mc, r.emit)
			out = append(out, r)
		}
	}
	if url := os.Getenv("TIDE_TRACCAR_URL"); url != "" {
		r := &Adapter{name: "traccar", started: time.Now()}
		r.emit = emitting(p, &r.count)
		r.src = traccaradapter.New(url,
			os.Getenv("TIDE_TRACCAR_USER"), os.Getenv("TIDE_TRACCAR_PASSWORD"),
			defaultTenant())
		out = append(out, r)
	}
	return out
}

// Run connects and subscribes one adapter; any failure is reflected in
// Health() (and therefore the next heartbeat) instead of crashing the engine.
func (r *Adapter) Run(ctx context.Context) {
	if err := r.src.Connect(ctx); err != nil {
		log.Printf("adapters: %s connect: %v", r.name, err)
		return
	}
	defer func() { _ = r.src.Disconnect(context.Background()) }()
	log.Printf("adapters: %s connected", r.name)
	var err error
	switch a := r.src.(type) {
	case *mqttadapter.Adapter:
		err = a.Subscribe(ctx, func(c context.Context, topic string, payload []byte) error {
			return a.HandleRaw(c, topic, payload)
		})
	case *traccaradapter.Adapter:
		err = a.Subscribe(ctx, func(c context.Context, _ string, payload []byte) error {
			t, herr := a.HandleRaw(payload, time.Now())
			if herr != nil {
				return herr
			}
			return r.emit(c, t)
		})
	default:
		log.Printf("adapters: %s: no subscribe path", r.name)
		return
	}
	if err != nil {
		log.Printf("adapters: %s subscribe: %v", r.name, err)
		return
	}
	// Subscribe returns after (un)subscribing — block here so the deferred
	// Disconnect only fires on shutdown, not immediately after subscribing.
	log.Printf("adapters: %s subscribed", r.name)
	<-ctx.Done()
}

func (r *Adapter) rate() float64 {
	el := time.Since(r.started).Seconds()
	if el <= 0 {
		return 0
	}
	return float64(r.count.Load()) / el
}

// Heartbeats publishes one heartbeat event per configured adapter carrying
// its real health. Returns the events for tests; pass publish to emit them.
func Heartbeats(ctx context.Context, now time.Time, regs []*Adapter, publish func(context.Context, events.Event) error) {
	for _, r := range regs {
		st := r.src.Health(ctx)
		_ = publish(ctx, events.Event{
			ID: "hb-" + r.name, Type: "tide.heartbeat.created",
			TenantID: "system", VehicleID: r.name, Timestamp: now,
			CorrelationID: "hb",
			Payload: map[string]any{
				"name": r.name, "state": string(st.State),
				"message": st.Message, "msgPerSec": r.rate(),
			},
			SchemaVersion: events.CurrentSchemaVersion,
		})
	}
}
