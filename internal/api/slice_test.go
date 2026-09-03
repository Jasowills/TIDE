package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tide-telematics/tide/internal/dedup"
	"github.com/tide-telematics/tide/internal/detectors"
	"github.com/tide-telematics/tide/internal/eventbus"
	"github.com/tide-telematics/tide/internal/geo"
	"github.com/tide-telematics/tide/internal/pipeline"
	"github.com/tide-telematics/tide/internal/rules"
	"github.com/tide-telematics/tide/internal/state"
	"github.com/tide-telematics/tide/internal/webhooks"
	"github.com/tide-telematics/tide/simulator/generators"
)

// Spec §7 vertical slice, in-process:
// FleetSim → HTTP → pipeline → state → speed rule → incident.created →
// webhook + console query surface (events API + vehicle state + triggers).
func TestVerticalSlice(t *testing.T) {
	// Webhook consumer: verifies HMAC like a real customer (docs guidance).
	var delivered [][]byte
	var hdrs []http.Header
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := webhooks.Verify("s3cr3t", r, body); err != nil {
			t.Errorf("consumer: bad webhook: %v", err)
			w.WriteHeader(400)
			return
		}
		delivered = append(delivered, body)
		hdrs = append(hdrs, r.Header)
		w.WriteHeader(200)
	}))
	defer consumer.Close()

	bus := &eventbus.MemoryBus{}
	eng := rules.NewEngine(webhooks.NewDispatcher())
	eng.Dispatcher.BaseDelay = time.Millisecond
	spec, err := rules.ParseSpec([]byte(`
id: speeding-alert
version: v1
when:
  eventType: vehicle.speeding.started
then:
  emit: incident.created
  webhook: ` + consumer.URL + `
  secret: s3cr3t
cooldownSecs: 0
maxActionsPerHour: 1000
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Publish(spec, time.Now()); err != nil {
		t.Fatal(err)
	}
	dcfg := detectors.DefaultConfig()
	dcfg.SpeedLimitKmh = 100
	dcfg.SpeedForSecs = 5
	p := &pipeline.Pipeline{
		Dedup: dedup.NewMemoryStore(0), States: state.NewMemoryStore(),
		Log: &pipeline.MemoryLog{}, Bus: bus,
		Detectors: detectors.NewTracker(dcfg), Rules: eng,
	}
	geostore := NewGeofenceStore(nil)
	srv := New(Deps{Pipeline: p, States: p.States, Bus: bus, Geo: geo.NewTracker(nil), Geofences: geostore, Rules: eng})
	api := httptest.NewServer(srv.Handler())
	defer api.Close()

	// FleetSim speeding window, indistinguishable from a real adapter.
	scen := generators.Scenario{Seed: 42, StepSecs: 5, DurationSecs: 120, Mix: map[string]float64{"speeding": 1}}
	scen.Start.Lat, scen.Start.Lng = 52.52, 13.405
	pts := generators.Generate(scen, 2, 42, "acme", time.Now().UTC())
	body, _ := json.Marshal(map[string]any{"batch": pts})
	resp, err := http.Post(api.URL+"/v1/telemetry:batch", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("ingest: want 202, got %d", resp.StatusCode)
	}

	// incident.created observable via console query surface…
	req, _ := http.NewRequest(http.MethodGet, api.URL+"/v1/events?tenant=acme&type=incident.created", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var evs []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&evs)
	resp.Body.Close()
	if len(evs) == 0 {
		t.Fatal("no incident.created observable in console API")
	}
	if evs[0]["ruleVersion"] != "v1" || evs[0]["causationId"] == "" {
		t.Fatalf("incident missing trace fields: %v", evs[0])
	}
	// …webhook delivered with valid signature…
	if len(delivered) == 0 {
		t.Fatal("no webhook delivered to consumer")
	}
	if hdrs[0].Get(webhooks.HeaderEventID) == "" {
		t.Fatal("webhook missing event id header")
	}
	// …vehicle state queryable…
	stResp, err := http.Get(api.URL + "/v1/vehicles/sim-000/state")
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]any
	_ = json.NewDecoder(stResp.Body).Decode(&st)
	stResp.Body.Close()
	if st["motion"] != "moving" {
		t.Fatalf("expected moving state, got %v", st)
	}
	// …and rule evaluation trace inspectable (Replay UI "why did this fire").
	trResp, err := http.Get(api.URL + "/v1/rules/triggers")
	if err != nil {
		t.Fatal(err)
	}
	var triggers []map[string]any
	_ = json.NewDecoder(trResp.Body).Decode(&triggers)
	trResp.Body.Close()
	if len(triggers) == 0 {
		t.Fatal("no rule evaluation trace")
	}
}
