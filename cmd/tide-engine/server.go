package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/tide-telematics/tide/adapters/registry"
	"github.com/tide-telematics/tide/internal/boot"
	"github.com/tide-telematics/tide/internal/config"
	"github.com/tide-telematics/tide/internal/observability"
	"github.com/tide-telematics/tide/internal/state"
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

	// Upstream adapters, env-configured (TIDE_MQTT_BROKER, TIDE_TRACCAR_URL…).
	// Each runs its own connect/subscribe loop; health flows to Connections.
	// Wiring lives in adapters/registry so provider names never appear in
	// engine code (provider-isolation lint).
	regs := registry.Configure(b.Pipeline)
	log.Printf("engine: %d adapter(s) configured", len(regs))
	for _, a := range regs {
		go a.Run(ctx)
	}

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
				heartbeat(ctx, b, now, regs)
			}
		}
	}()

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

func heartbeat(ctx context.Context, b *boot.Bundle, now time.Time, regs []*registry.Adapter) {
	_ = b.Pipeline.Bus.Publish(ctx, events.Event{
		ID: "hb-tide-engine", Type: "tide.heartbeat.created",
		TenantID: "system", VehicleID: "tide-engine", Timestamp: now,
		CorrelationID: "hb",
		Payload: map[string]any{"name": "tide-engine", "state": "HEALTHY"},
		SchemaVersion: events.CurrentSchemaVersion,
	})
	registry.Heartbeats(ctx, now, regs, b.Pipeline.Bus.Publish)
}
