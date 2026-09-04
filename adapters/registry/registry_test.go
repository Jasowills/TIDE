package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tide-telematics/tide/internal/pipeline"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
	"github.com/tide-telematics/tide/schemas/events"
)

// The registry runs exactly the adapters with credentials present — nothing
// fabricated, nothing silently skipped.
func TestConfigureMatrix(t *testing.T) {
	p, _, _ := pipeline.NewTestPipeline()
	t.Setenv("TIDE_MQTT_BROKER", "")
	t.Setenv("TIDE_TRACCAR_URL", "")
	if got := Configure(p); len(got) != 0 {
		t.Fatalf("no credentials → no adapters, got %d", len(got))
	}
	t.Setenv("TIDE_TRACCAR_URL", "http://traccar.local:8082")
	t.Setenv("TIDE_TRACCAR_USER", "admin")
	t.Setenv("TIDE_TRACCAR_PASSWORD", "admin")
	got := Configure(p)
	if len(got) != 1 || got[0].Name() != "traccar" {
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
	p, bus, _ := pipeline.NewTestPipeline()
	regs := Configure(p)
	if len(regs) != 1 {
		t.Fatalf("want 1 adapter, got %d", len(regs))
	}
	r := regs[0]
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := r.Source().Connect(ctx); err != nil {
		t.Fatalf("connect to fixture upstream: %v", err)
	}
	raw := `{"id":18341,"deviceId":7,"protocol":"teltonika","deviceTime":"2026-02-10T08:00:00Z","fixTime":"2026-02-10T08:00:00Z","latitude":52.52,"longitude":13.405,"speed":48.6,"attributes":{"ignition":true}}`
	tel, err := r.Source().(interface {
		HandleRaw([]byte, time.Time) (ctelemetry.Telemetry, error)
	}).HandleRaw([]byte(raw), time.Now())
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if tel.TenantID != "wired" {
		t.Fatalf("tenant not carried: %q", tel.TenantID)
	}
	if _, err := p.Process(ctx, tel); err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	if len(bus.Events) == 0 {
		t.Fatal("no bus events from wired adapter")
	}
	// Heartbeats reflect the real adapter state.
	var beats []events.Event
	Heartbeats(ctx, time.Now(), regs, func(_ context.Context, e events.Event) error {
		beats = append(beats, e)
		return nil
	})
	found := false
	for _, e := range beats {
		if e.Type != "tide.heartbeat.created" {
			continue
		}
		var pl map[string]any
		raw, _ := json.Marshal(e.Payload)
		_ = json.Unmarshal(raw, &pl)
		if pl["name"] == "traccar" && pl["state"] == "HEALTHY" {
			found = true
		}
	}
	if !found {
		t.Fatal("no HEALTHY traccar heartbeat")
	}
}
