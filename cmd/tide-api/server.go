package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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
		Registry: registry, Limiter: rateLimiterFromEnv(),
	})
	if b.NATSUp {
		// Resilient forward: survives NATS outages without api restart.
		eventbus.SubscribeResilient(ctx, cfg.NATS.URL, "tide.events.>", func(raw []byte) {
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
	}
	httpSrv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.APIPort), Handler: srv.Handler()}
	go func() {
		<-ctx.Done()
		_ = httpSrv.Shutdown(context.Background())
	}()
	// Event-bus health in Connections (ADV-0005): the bus wrapper remembers
	// publish failures, so the console shows NATS trouble even when a fresh
	// dial (doctor) would succeed.
	if b.NATSBus != nil {
		go func() {
			t := time.NewTicker(30 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					ts, msg := b.NATSBus.LastError()
					if !ts.IsZero() && time.Since(ts) < 10*time.Minute {
						registry.Set("eventbus", adapters.HealthStatus{State: adapters.StateDegraded, Message: msg})
					} else {
						registry.Set("eventbus", adapters.HealthStatus{State: adapters.StateHealthy})
					}
				}
			}
		}()
	}
	log.Printf("tide-api listening on :%d (env=%s)", cfg.APIPort, cfg.Env)
	return httpSrv.ListenAndServe()
}

// rateLimiterFromEnv honors TIDE_RATE_LIMIT_PER_MIN (requests/min/IP).
// 0 disables the limiter; unset keeps the 600/min default. Load-test and
// perf CI raise it so latency budgets measure the pipeline, not 429s — the
// limiter itself stays covered by the Go burst test (A06).
func rateLimiterFromEnv() *tideapi.RateLimiter {
	v := strings.TrimSpace(os.Getenv("TIDE_RATE_LIMIT_PER_MIN"))
	if v == "" {
		return nil // New() installs the default
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		log.Printf("api: bad TIDE_RATE_LIMIT_PER_MIN %q, using default", v)
		return nil
	}
	if n == 0 {
		return tideapi.NewRateLimiter(0, time.Minute) // disabled
	}
	return tideapi.NewRateLimiter(n, time.Minute)
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
