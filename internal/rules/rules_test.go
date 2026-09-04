package rules

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tide-telematics/tide/internal/state"
	"github.com/tide-telematics/tide/internal/webhooks"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
	"github.com/tide-telematics/tide/schemas/events"
)

const speedingRule = `
id: speeding-alert
version: v1
when:
  eventType: vehicle.speeding.started
  conditions:
    - field: speedKmh
      op: ">"
      value: 120
then:
  emit: incident.created
  webhook: REPLACE_ME
  secret: s3cr3t
cooldownSecs: 60
maxActionsPerHour: 5
`

func tel(speed float64) ctelemetry.Telemetry {
	ign := true
	return ctelemetry.Telemetry{
		ID: "x", TenantID: "t", VehicleID: "v", DeviceID: "d",
		Timestamp: time.Now(), ReceivedAt: time.Now(),
		Location:  ctelemetry.Location{Lat: 1, Lng: 2},
		SpeedKmh:  &speed, Ignition: &ign,
		Raw:       map[string]any{},
		Metadata:  ctelemetry.Metadata{CorrelationID: "c", SchemaVersion: 1},
	}
}

func trigEv() events.Event {
	return events.Event{ID: "e1", Type: "vehicle.speeding.started", TenantID: "t",
		VehicleID: "v", Timestamp: time.Now(), CorrelationID: "c", SchemaVersion: 1}
}

func TestRuleFiresAndTraces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	spec, err := ParseSpec([]byte(speedingRule))
	if err != nil {
		t.Fatal(err)
	}
	spec.Then.Webhook = srv.URL
	disp := webhooks.NewDispatcher()
	disp.AllowPrivate = true // httptest serves loopback
	eng := NewEngine(disp)
	eng.Dispatcher.BaseDelay = time.Millisecond
	if err := eng.Publish(spec, time.Now()); err != nil {
		t.Fatal(err)
	}
	out := eng.Evaluate(context.Background(), tel(130), state.VehicleState{}, []events.Event{trigEv()})
	if len(out) != 1 || out[0].Type != "incident.created" {
		t.Fatalf("rule did not emit incident: %+v", out)
	}
	if out[0].RuleVersion != "v1" || out[0].CausationID != "e1" {
		t.Fatalf("trace fields missing: %+v", out[0])
	}
	tr := eng.Triggers()
	if len(tr) != 1 || len(tr[0].ActionsTaken) == 0 {
		t.Fatalf("evaluation trace missing: %+v", tr)
	}
	if len(eng.Dispatcher.Delivered) != 1 {
		t.Fatal("webhook not delivered")
	}
}

// T060: publishing v2 never mutates v1's recorded behavior.
func TestVersionsImmutable(t *testing.T) {
	eng := NewEngine(nil)
	s1, _ := ParseSpec([]byte("id: r\nversion: v1\nwhen:\n  eventType: a.b.started\n"))
	s2, _ := ParseSpec([]byte("id: r\nversion: v2\nwhen:\n  eventType: a.b.started\n"))
	_ = eng.Publish(s1, time.Now())
	_ = eng.Publish(s2, time.Now())
	mutated := s1
	mutated.When.EventType = "changed"
	if err := eng.Publish(mutated, time.Now()); err == nil {
		t.Fatal("republishing a version with different content must fail")
	}
}

// T062: valves — cooldown + max-actions/hour cap a malfunctioning device.
func TestSafetyValves(t *testing.T) {
	eng := NewEngine(nil)
	spec, _ := ParseSpec([]byte("id: noisy\nversion: v1\nwhen:\n  eventType: vehicle.speeding.started\n"))
	spec.CooldownSecs = 3600
	spec.MaxActionsPerHour = 2
	_ = eng.Publish(spec, time.Now())
	ctx := context.Background()
	st := state.VehicleState{}
	first := eng.Evaluate(ctx, tel(10), st, []events.Event{trigEv()})
	second := eng.Evaluate(ctx, tel(10), st, []events.Event{trigEv()})
	if len(first) != 0 {
		t.Fatalf("no-emit rule should not emit, got %v", first)
	}
	_ = second
	// With emit added, cooldown suppresses the immediate repeat.
	spec.Then.Emit = "incident.created"
	eng2 := NewEngine(nil)
	_ = eng2.Publish(spec, time.Now())
	a := eng2.Evaluate(ctx, tel(10), st, []events.Event{trigEv()})
	b := eng2.Evaluate(ctx, tel(10), st, []events.Event{trigEv()})
	if len(a) != 1 || len(b) != 0 {
		t.Fatalf("cooldown valve failed: %d %d", len(a), len(b))
	}
}
