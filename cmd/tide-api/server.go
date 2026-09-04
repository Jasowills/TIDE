package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	tideapi "github.com/tide-telematics/tide/internal/api"
	"github.com/tide-telematics/tide/adapters"
	"github.com/tide-telematics/tide/internal/boot"
	"github.com/tide-telematics/tide/internal/config"
	"github.com/tide-telematics/tide/internal/eventbus"
	"github.com/tide-telematics/tide/internal/observability"
	"github.com/tide-telematics/tide/schemas/events"
)

// runAPI boots the bundle, serves REST+WS, and (when NATS is up) forwards
// engine-originated events onto the WebSocket hub + Postgres.
func runAPI(ctx context.Context, cfg config.Config) error {
	shutdown, err := observability.Init(ctx, "tide-api")
	if err != nil {
		return err
	}
	defer func() { _ = shutdown(context.Background()) }()

	b := boot.Build(ctx, cfg)
	registry := tideapi.NewAdapterRegistry()
	srv := tideapi.New(tideapi.Deps{
		Pipeline: b.Pipeline, States: b.States, Bus: b.MemBus, PG: b.PG,
		Geo: b.Geo, Geofences: tideapi.NewGeofenceStore(b.PG), Rules: b.Rules,
		Registry: registry,
	})
	if b.NATSUp {
		unsub, err := eventbus.Subscribe(cfg.NATS.URL, "tide.events.>", func(raw []byte) {
			var e events.Event
			if err := json.Unmarshal(raw, &e); err != nil {
				return
			}
			// Heartbeats update the Connections registry; domain events fan out.
			if e.Type == "tide.heartbeat.created" {
				name, _ := e.Payload["name"].(string)
				st, _ := e.Payload["state"].(string)
				if name != "" {
					registry.Set(name, heartbeatStatus(st, "via tide.heartbeat"))
				}
				return
			}
			srv.Inject(e)
		})
		if err != nil {
			log.Printf("api: nats forward failed: %v", err)
		} else {
			defer unsub()
		}
	}
	httpSrv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.APIPort), Handler: srv.Handler()}
	go func() {
		<-ctx.Done()
		_ = httpSrv.Shutdown(context.Background())
	}()
	log.Printf("tide-api listening on :%d (env=%s)", cfg.APIPort, cfg.Env)
	return httpSrv.ListenAndServe()
}

func heartbeatStatus(state, msg string) adapters.HealthStatus {
	st := adapters.HealthStatus{State: adapters.HealthState(state), Message: msg}
	switch st.State {
	case adapters.StateHealthy, adapters.StateConfigured, adapters.StateConnecting,
		adapters.StateDegraded, adapters.StateReconnecting, adapters.StateFailed:
	default:
		st.State = adapters.StateConfigured
	}
	return st
}
