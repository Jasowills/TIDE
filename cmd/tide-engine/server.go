package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/tide-telematics/tide/internal/boot"
	"github.com/tide-telematics/tide/internal/config"
	"github.com/tide-telematics/tide/internal/observability"
	"github.com/tide-telematics/tide/internal/state"
	mqttadapter "github.com/tide-telematics/tide/adapters/mqtt"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
	"github.com/tide-telematics/tide/schemas/events"
)

// runEngine serves health, runs the offline sweeper (per-device cadence,
// T032) and the MQTT subscriber when configured (T021 live path).
func runEngine(ctx context.Context, cfg config.Config) error {
	shutdown, err := observability.Init(ctx, "tide-engine")
	if err != nil {
		return err
	}
	defer func() { _ = shutdown(context.Background()) }()

	b := boot.Build(ctx, cfg)

	// Offline sweeper: presence transitions for silent devices.
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				sweep(ctx, b, now)
				heartbeat(ctx, b, now)
			}
		}
	}()

	// MQTT live path (optional in V1: TIDE_MQTT_BROKER + TIDE_MQTT_MAPPING).
	if broker := os.Getenv("TIDE_MQTT_BROKER"); broker != "" {
		go runMQTT(ctx, b, cfg, broker)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ready")) })
	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.EnginePort), Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	log.Printf("tide-engine listening on :%d (env=%s)", cfg.EnginePort, cfg.Env)
	return srv.ListenAndServe()
}

func sweep(ctx context.Context, b *boot.Bundle, now time.Time) {
	lister, ok := b.States.(boot.Lister)
	if !ok {
		return
	}
	ids, err := lister.ListIDs(ctx)
	if err != nil {
		return
	}
	for _, id := range ids {
		s, found, err := b.States.Get(ctx, id)
		if err != nil || !found {
			continue
		}
		next, tr := state.CheckOffline(s, 0, now) // cadence from vehicle record post-V1; default 300s
		if tr == nil {
			continue
		}
		_ = b.States.Set(ctx, next)
		_ = b.Pipeline.Bus.Publish(ctx, events.Event{
			ID: events.DeterministicID("vehicle.offline.started", id, now, "sweep"),
			Type: "vehicle.offline.started", TenantID: s.TenantID, VehicleID: id,
			Timestamp: now, CorrelationID: "sweep", SchemaVersion: events.CurrentSchemaVersion,
		})
	}
}

func heartbeat(ctx context.Context, b *boot.Bundle, now time.Time) {
	mqttState := "CONFIGURED"
	if os.Getenv("TIDE_MQTT_BROKER") != "" {
		mqttState = "HEALTHY"
	}
	for name, st := range map[string]string{"tide-engine": "HEALTHY", "mqtt": mqttState} {
		_ = b.Pipeline.Bus.Publish(ctx, events.Event{
			ID: "hb-" + name, Type: "tide.heartbeat.created",
			TenantID: "system", VehicleID: name, Timestamp: now,
			CorrelationID: "hb",
			Payload: map[string]any{"name": name, "state": st},
			SchemaVersion: events.CurrentSchemaVersion,
		})
	}
}

func runMQTT(ctx context.Context, b *boot.Bundle, cfg config.Config, broker string) {
	mapPath := os.Getenv("TIDE_MQTT_MAPPING")
	if mapPath == "" {
		mapPath = "examples/mqtt-mapping.yaml"
	}
	raw, err := os.ReadFile(mapPath)
	if err != nil {
		log.Printf("engine: mqtt mapping missing (%v) — subscriber off", err)
		return
	}
	mc, err := mqttadapter.LoadMapping(raw)
	if err != nil {
		log.Printf("engine: bad mqtt mapping: %v", err)
		return
	}
	mc.Broker = broker
	a := mqttadapter.New(mc, func(c context.Context, t ctelemetry.Telemetry) error {
		_, err := b.Pipeline.Process(c, t)
		return err
	})
	if err := a.Connect(ctx); err != nil {
		log.Printf("engine: mqtt connect: %v", err)
		return
	}
	defer func() { _ = a.Disconnect(context.Background()) }()
	err = a.Subscribe(ctx, func(c context.Context, topic string, payload []byte) error {
		return a.HandleRaw(c, topic, payload)
	})
	if err != nil {
		log.Printf("engine: mqtt subscribe: %v", err)
		return
	}
	log.Printf("engine: mqtt subscribed to %v", mc.Topics)
	<-ctx.Done()
}
