package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tide-telematics/tide/internal/boot"
	"github.com/tide-telematics/tide/internal/config"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

// The engine runs exactly the adapters with credentials present — nothing
// fabricated, nothing silently skipped.
func TestConfiguredAdaptersMatrix(t *testing.T) {
	b := boot.Build(context.Background(), configForAdapterTest(t))
	t.Setenv("TIDE_MQTT_BROKER", "")
	t.Setenv("TIDE_TRACCAR_URL", "")
	if got := configuredAdapters(b); len(got) != 0 {
		t.Fatalf("no credentials → no adapters, got %d", len(got))
	}
	t.Setenv("TIDE_TRACCAR_URL", "http://traccar.local:8082")
	t.Setenv("TIDE_TRACCAR_USER", "admin")
	t.Setenv("TIDE_TRACCAR_PASSWORD", "admin")
	got := configuredAdapters(b)
	if len(got) != 1 || got[0].name != "traccar" {
		t.Fatalf("traccar env → one traccar adapter, got %+v", got)
	}
}

// Full wiring proof with a fixture Traccar upstream: records → adapter →
// production pipeline → bus event. No live Traccar needed.
func TestTraccarWiringEndToEnd(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/server":
			_, _ = w.Write([]byte(`{"version":"6.5.0"}`))
		case "/api/positions":
			_, _ = w.Write([]byte(`[{
				"id": 18341, "deviceId": 7, "protocol": "teltonika",
				"deviceTime": "2026-02-10T08:00:00Z", "fixTime": "2026-02-10T08:00:00Z",
				"latitude": 52.52, "longitude": 13.405, "speed": 48.6,
				"attributes": {"ignition": true}}]`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()

	t.Setenv("TIDE_TRACCAR_URL", upstream.URL)
	t.Setenv("TIDE_TRACCAR_USER", "admin")
	t.Setenv("TIDE_TRACCAR_PASSWORD", "admin")
	t.Setenv("TIDE_TENANT", "wired")
	b := boot.Build(context.Background(), configForAdapterTest(t))
	regs := configuredAdapters(b)
	if len(regs) != 1 {
		t.Fatalf("want 1 adapter, got %d", len(regs))
	}
	r := regs[0]
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := r.src.Connect(ctx); err != nil {
		t.Fatalf("connect to fixture upstream: %v", err)
	}
	// One poll cycle, driven directly (the 15s ticker belongs to Subscribe).
	raw := `{"id":18341,"deviceId":7,"protocol":"teltonika","deviceTime":"2026-02-10T08:00:00Z","fixTime":"2026-02-10T08:00:00Z","latitude":52.52,"longitude":13.405,"speed":48.6,"attributes":{"ignition":true}}`
	tel, err := r.src.(interface {
		HandleRaw([]byte, time.Time) (ctelemetry.Telemetry, error)
	}).HandleRaw([]byte(raw), time.Now())
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if tel.TenantID != "wired" {
		t.Fatalf("tenant not carried: %q", tel.TenantID)
	}
	if _, err := b.Pipeline.Process(ctx, tel); err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	if len(b.MemBus.Events) == 0 {
		t.Fatal("no bus events from wired adapter")
	}
	// Heartbeat reflects the real adapter state.
	heartbeatAdapters(ctx, b, time.Now(), regs)
	found := false
	for _, e := range b.MemBus.Events {
		if e.Type == "tide.heartbeat.created" {
			var p map[string]any
			raw, _ := json.Marshal(e.Payload)
			_ = json.Unmarshal(raw, &p)
			if p["name"] == "traccar" && p["state"] == "HEALTHY" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("no HEALTHY traccar heartbeat")
	}
}

func configForAdapterTest(t *testing.T) config.Config {
	t.Helper()
	// Hermetic: unreachable backends force memory fallbacks (fast refused
	// connections, no hangs).
	t.Setenv("TIDE_POSTGRES_DSN", "postgres://127.0.0.1:15432/tide?sslmode=disable")
	t.Setenv("TIDE_REDIS_ADDR", "127.0.0.1:16399")
	t.Setenv("TIDE_NATS_URL", "nats://127.0.0.1:14222")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
