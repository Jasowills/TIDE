package pipeline

import (
	"context"
	"testing"
	"time"

	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

func tel(id string, ts time.Time, speed float64, ign bool, seq *int64) ctelemetry.Telemetry {
	return ctelemetry.Telemetry{
		ID: id, TenantID: "t", VehicleID: "v", DeviceID: "d",
		Timestamp: ts, ReceivedAt: ts,
		Location:  ctelemetry.Location{Lat: 52.5, Lng: 13.4},
		SpeedKmh:  &speed, Ignition: &ign,
		Raw:       map[string]any{"seq": id},
		Source:    ctelemetry.Source{Provider: "sim", Protocol: "mqtt", DeviceID: "d"},
		Metadata:  ctelemetry.Metadata{CorrelationID: "c", SchemaVersion: 1, Sequence: seq, Quality: "good"},
	}
}

func seqptr(s int64) *int64 { return &s }

// QA §3.1.5 property: duplicate telemetry → no duplicate state transition.
func TestDuplicateYieldsNothing(t *testing.T) {
	ctx := context.Background()
	p, bus, _ := NewTestPipeline()
	base := time.Now()
	if _, err := p.Process(ctx, tel("a", base, 60, true, seqptr(1))); err != nil {
		t.Fatal(err)
	}
	n := len(bus.Events)
	// Same sequence → dedup hit → zero new events, zero state churn.
	evs, err := p.Process(ctx, tel("a-dup", base, 60, true, seqptr(1)))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 0 || len(bus.Events) != n {
		t.Fatalf("duplicate produced output: %d new events", len(bus.Events)-n)
	}
}

// Property: out-of-order arrival → correct final state regardless of order.
func TestOutOfOrderFinalState(t *testing.T) {
	ctx := context.Background()
	base := time.Now()
	mk := func() *Pipeline { p, _, _ := NewTestPipeline(); return p }
	ordered := []ctelemetry.Telemetry{
		tel("1", base, 0, false, seqptr(1)),
		tel("2", base.Add(time.Second), 80, true, seqptr(2)),
		tel("3", base.Add(2*time.Second), 90, true, seqptr(3)),
	}
	p1 := mk()
	for _, x := range ordered {
		if _, err := p1.Process(ctx, x); err != nil {
			t.Fatal(err)
		}
	}
	s1, _, _ := p1.States.Get(ctx, "v")

	shuffled := []ctelemetry.Telemetry{ordered[2], ordered[0], ordered[1]}
	p2 := mk()
	for _, x := range shuffled {
		if _, err := p2.Process(ctx, x); err != nil {
			t.Fatal(err)
		}
	}
	s2, _, _ := p2.States.Get(ctx, "v")
	if s1.Motion != s2.Motion || !s1.LastSeen.Equal(s2.LastSeen) {
		t.Fatalf("order affected final state: %+v vs %+v", s1, s2)
	}
}
