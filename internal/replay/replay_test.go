package replay

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/tide-telematics/tide/internal/detectors"
	"github.com/tide-telematics/tide/internal/pipeline"
	"github.com/tide-telematics/tide/internal/rules"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

// Fixed historical fixture: speeding window. Determinism test (T071):
// same fixture → identical event sequence, byte-for-byte, in CI.
func speedingWindow(base time.Time) []ctelemetry.Telemetry {
	var pts []ctelemetry.Telemetry
	speeds := []float64{40, 45, 110, 115, 120, 118, 40, 30}
	for i, s := range speeds {
		ign := true
		pts = append(pts, ctelemetry.Telemetry{
			ID: string(rune('a' + i)), TenantID: "t", VehicleID: "v1", DeviceID: "d1",
			Timestamp: base.Add(time.Duration(i*5) * time.Second),
			ReceivedAt: base.Add(time.Duration(i*5) * time.Second),
			Location:  ctelemetry.Location{Lat: 52.5, Lng: 13.4},
			SpeedKmh:  &s, Ignition: &ign,
			Raw:       map[string]any{},
			Source:    ctelemetry.Source{Provider: "sim", Protocol: "mqtt", DeviceID: "d1"},
			Metadata:  ctelemetry.Metadata{CorrelationID: "replay-fixture", SchemaVersion: 1},
		})
	}
	return pts
}

func buildWithRule(ruleYAML string) func() *pipeline.Pipeline {
	return func() *pipeline.Pipeline {
		p, _, _ := pipeline.NewTestPipeline()
		cfg := detectors.DefaultConfig()
		cfg.SpeedLimitKmh = 100
		cfg.SpeedForSecs = 5
		p.Detectors = detectors.NewTracker(cfg)
		if ruleYAML != "" {
			spec, err := rules.ParseSpec([]byte(ruleYAML))
			if err != nil {
				panic(err)
			}
			eng := rules.NewEngine(nil)
			if err := eng.Publish(spec, time.Now()); err != nil {
				panic(err)
			}
			p.Rules = eng
		}
		return p
	}
}

const incidentRule = `
id: speeding-alert
version: v1
when:
  eventType: vehicle.speeding.started
then:
  emit: incident.created
`

func TestReplayDeterministic(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	build := buildWithRule(incidentRule)
	r1, err := Run(ctx, speedingWindow(base), build)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Run(ctx, speedingWindow(base), build)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(eventTypes(r1.Events), eventTypes(r2.Events)) {
		t.Fatalf("non-deterministic:\n%v\n%v", eventTypes(r1.Events), eventTypes(r2.Events))
	}
	// Byte-for-byte: ids included (deterministic ids from event time+cause).
	var ids1, ids2 []string
	for _, e := range r1.Events {
		ids1 = append(ids1, e.ID)
	}
	for _, e := range r2.Events {
		ids2 = append(ids2, e.ID)
	}
	if !reflect.DeepEqual(ids1, ids2) {
		t.Fatalf("event ids differ:\n%v\n%v", ids1, ids2)
	}
	found := map[string]bool{}
	for _, x := range eventTypes(r1.Events) {
		found[x] = true
	}
	for _, want := range []string{"vehicle.speeding.started", "incident.created", "vehicle.speeding.ended"} {
		if !found[want] {
			t.Fatalf("missing %s in %v", want, eventTypes(r1.Events))
		}
	}
}

// T072: rule-version comparison over the same window.
func TestCompareRuleVersions(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	pts := speedingWindow(base)
	v1 := buildWithRule(incidentRule)
	v2 := buildWithRule(`
id: speeding-alert
version: v2
when:
  eventType: vehicle.speeding.started
  conditions:
    - field: speedKmh
      op: ">"
      value: 200
then:
  emit: incident.created
`)
	a, b, err := Compare(ctx, pts, v1, v2)
	if err != nil {
		t.Fatal(err)
	}
	has := func(seq []string, x string) bool {
		for _, s := range seq {
			if s == x {
				return true
			}
		}
		return false
	}
	if !has(a, "incident.created") {
		t.Fatalf("v1 should fire incident: %v", a)
	}
	if has(b, "incident.created") {
		t.Fatalf("v2 (limit 200) should not fire incident: %v", b)
	}
}
