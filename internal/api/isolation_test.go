package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tide-telematics/tide/internal/dedup"
	"github.com/tide-telematics/tide/internal/detectors"
	"github.com/tide-telematics/tide/internal/eventbus"
	"github.com/tide-telematics/tide/internal/geo"
	"github.com/tide-telematics/tide/internal/pipeline"
	"github.com/tide-telematics/tide/internal/state"
)

// T112: tenant isolation is enforced at the query layer — tenant B can never
// read tenant A's events through the console API.
func TestTenantIsolation(t *testing.T) {
	bus := &eventbus.MemoryBus{}
	p := &pipeline.Pipeline{
		Dedup: dedup.NewMemoryStore(0), States: state.NewMemoryStore(),
		Log: &pipeline.MemoryLog{}, Bus: bus,
		Detectors: detectors.NewTracker(detectors.DefaultConfig()),
	}
	srv := New(Deps{Pipeline: p, States: p.States, Bus: bus,
		Geo: geo.NewTracker(nil), Geofences: NewGeofenceStore(nil), Rules: nil})
	api := httptest.NewServer(srv.Handler())
	defer api.Close()

	post := func(tenant string) {
		payload := map[string]any{
			"tenantId": tenant, "vehicleId": "v1", "deviceId": "d",
			"timestamp": "2026-01-01T00:00:00Z",
			"location":  map[string]any{"lat": 1, "lng": 2},
			"speed":     10.0,
			"raw":       map[string]any{},
			"source":    map[string]any{"provider": "x", "protocol": "http", "deviceId": "d"},
			"metadata":  map[string]any{"correlationId": "c", "schemaVersion": 1, "quality": "good"},
		}
		raw, _ := json.Marshal(payload)
		resp, err := http.Post(api.URL+"/v1/telemetry", "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("ingest %s: %d", tenant, resp.StatusCode)
		}
	}
	post("acme")
	post("other")

	get := func(tenant string) []map[string]any {
		resp, err := http.Get(api.URL + "/v1/events?tenant=" + tenant)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out []map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}
	for _, e := range get("acme") {
		if e["tenantId"] != "acme" {
			t.Fatalf("tenant leak: %v", e)
		}
	}
	for _, e := range get("other") {
		if e["tenantId"] != "other" {
			t.Fatalf("tenant leak: %v", e)
		}
	}
	// No tenant param → rejected, never unfiltered.
	resp, err := http.Get(api.URL + "/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unscoped query must be rejected, got %d", resp.StatusCode)
	}
}
