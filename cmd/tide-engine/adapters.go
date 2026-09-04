package main

import (
	"context"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/tide-telematics/tide/adapters"
	traccaradapter "github.com/tide-telematics/tide/adapters/traccar"
	mqttadapter "github.com/tide-telematics/tide/adapters/mqtt"
	"github.com/tide-telematics/tide/internal/boot"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
	"github.com/tide-telematics/tide/schemas/events"
)

// runningAdapter is one env-configured upstream source. The engine only runs
// adapters with credentials present — nothing is fabricated: an unconfigured
// adapter simply doesn't appear in Connections, and heartbeats report the
// adapter's ACTUAL Health(), never env-presence.
type runningAdapter struct {
	name    string
	src     adapters.TelemetrySource
	emit    func(ctx context.Context, t ctelemetry.Telemetry) error
	count   atomic.Int64
	started time.Time
}

func defaultTenant() string {
	if v := os.Getenv("TIDE_TENANT"); v != "" {
		return v
	}
	return "default"
}

func (r *runningAdapter) emitting(b *boot.Bundle) func(ctx context.Context, t ctelemetry.Telemetry) error {
	return func(ctx context.Context, t ctelemetry.Telemetry) error {
		evs, err := b.Pipeline.Process(ctx, t)
		if err != nil {
			return err
		}
		r.count.Add(1)
		_ = evs
		return nil
	}
}

func (r *runningAdapter) rate() float64 {
	el := time.Since(r.started).Seconds()
	if el <= 0 {
		return 0
	}
	return float64(r.count.Load()) / el
}

// configuredAdapters builds every upstream with credentials present.
// flespi is intentionally absent: its live transport is still a stub
// (Subscribe blocks without subscribing) — advertising it HEALTHY would be
// dishonest. It ships as normalization + contract tests until the live path
// lands.
func configuredAdapters(b *boot.Bundle) []*runningAdapter {
	var out []*runningAdapter
	if broker := os.Getenv("TIDE_MQTT_BROKER"); broker != "" {
		mapPath := os.Getenv("TIDE_MQTT_MAPPING")
		if mapPath == "" {
			mapPath = "examples/mqtt-mapping.yaml"
		}
		raw, err := os.ReadFile(mapPath)
		if err != nil {
			log.Printf("engine: mqtt mapping missing (%v) — subscriber off", err)
		} else if mc, err := mqttadapter.LoadMapping(raw); err != nil {
			log.Printf("engine: bad mqtt mapping: %v", err)
		} else {
			mc.Broker = broker
			r := &runningAdapter{name: "mqtt", started: time.Now()}
			r.emit = r.emitting(b)
			r.src = mqttadapter.New(mc, r.emit)
			out = append(out, r)
		}
	}
	if url := os.Getenv("TIDE_TRACCAR_URL"); url != "" {
		r := &runningAdapter{name: "traccar", started: time.Now()}
		r.emit = r.emitting(b)
		r.src = traccaradapter.New(url,
			os.Getenv("TIDE_TRACCAR_USER"), os.Getenv("TIDE_TRACCAR_PASSWORD"),
			defaultTenant())
		out = append(out, r)
	}
	return out
}

// run connects and subscribes one adapter; any failure is reflected in
// Health() (and therefore the next heartbeat) instead of crashing the engine.
func (r *runningAdapter) run(ctx context.Context, b *boot.Bundle) {
	if err := r.src.Connect(ctx); err != nil {
		log.Printf("engine: %s connect: %v", r.name, err)
		return
	}
	defer func() { _ = r.src.Disconnect(context.Background()) }()
	log.Printf("engine: %s connected", r.name)
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
		log.Printf("engine: %s: no subscribe path", r.name)
		return
	}
	if err != nil {
		log.Printf("engine: %s subscribe: %v", r.name, err)
		return
	}
	// Subscribe returns after (un)subscribing — block here so the deferred
	// Disconnect only fires on shutdown, not immediately after subscribing.
	log.Printf("engine: %s subscribed", r.name)
	<-ctx.Done()
}

// heartbeatAdapters publishes each configured adapter's real health.
func heartbeatAdapters(ctx context.Context, b *boot.Bundle, now time.Time, regs []*runningAdapter) {
	for _, r := range regs {
		st := r.src.Health(ctx)
		_ = b.Pipeline.Bus.Publish(ctx, events.Event{
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
