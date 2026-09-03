package state

import (
	"context"
	"math/rand"
	"testing"
	"time"

	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

func tel(ts time.Time, speed *float64, ign *bool) ctelemetry.Telemetry {
	return ctelemetry.Telemetry{
		TenantID: "t", VehicleID: "v", DeviceID: "d",
		Timestamp: ts, ReceivedAt: ts,
		Location:  ctelemetry.Location{Lat: 1, Lng: 2},
		SpeedKmh:  speed, Ignition: ign,
		Raw:       map[string]any{},
		Metadata:  ctelemetry.Metadata{SchemaVersion: ctelemetry.CurrentSchemaVersion},
	}
}

func fptr(f float64) *float64 { return &f }
func bptr(b bool) *bool       { return &b }

// T031 acceptance: no repeated identical-state transitions.
func TestNoRepeatedIdenticalTransitions(t *testing.T) {
	var eng Engine
	var cur VehicleState
	base := time.Now()
	// 50 identical stopped points → at most ONE motion transition total.
	n := 0
	for i := 0; i < 50; i++ {
		var next VehicleState
		var tr []Transition
		next, tr = eng.Apply(cur, tel(base.Add(time.Duration(i)*time.Second), fptr(0), bptr(false)))
		cur = next
		for _, x := range tr {
			if x.Dimension == "motion" {
				n++
			}
		}
	}
	if n > 1 {
		t.Fatalf("repeated identical-state transitions: %d", n)
	}
}

// Property: random telemetry streams never emit back-to-back identical transitions.
func TestPropertyNoFlapping(t *testing.T) {
	var eng Engine
	r := rand.New(rand.NewSource(7))
	for trial := 0; trial < 20; trial++ {
		var cur VehicleState
		last := map[string]string{}
		base := time.Now()
		for i := 0; i < 200; i++ {
			s := r.Float64() * 120
			ign := r.Float64() > 0.3
			next, tr := eng.Apply(cur, tel(base.Add(time.Duration(i)*time.Second), &s, &ign))
			cur = next
			for _, x := range tr {
				if last[x.Dimension] == x.To && x.From == x.To {
					t.Fatalf("self-transition %v", x)
				}
				last[x.Dimension] = x.To
			}
		}
	}
}

// T032: per-device cadence — 5-min device silent 30s is NOT offline.
func TestOfflinePerDeviceCadence(t *testing.T) {
	now := time.Now()
	s := VehicleState{VehicleID: "v", Presence: PresenceOnline, LastSeen: now.Add(-30 * time.Second)}
	if _, tr := CheckOffline(s, 300, now); tr != nil {
		t.Fatal("30s silence on 300s-cadence device must not be offline")
	}
	s2 := VehicleState{VehicleID: "v", Presence: PresenceOnline, LastSeen: now.Add(-16 * time.Minute)}
	if _, tr := CheckOffline(s2, 300, now); tr == nil {
		t.Fatal("16min silence on 300s-cadence device must be offline")
	}
	// Fast 10s-cadence device silent 60s IS offline.
	s3 := VehicleState{VehicleID: "v", Presence: PresenceOnline, LastSeen: now.Add(-60 * time.Second)}
	if _, tr := CheckOffline(s3, 10, now); tr == nil {
		t.Fatal("60s silence on 10s-cadence device must be offline")
	}
}

// T030: Redis dies → rebuild from durable telemetry.
func TestRebuildFromLog(t *testing.T) {
	ctx := context.Background()
	base := time.Now()
	log := []ctelemetry.Telemetry{
		tel(base, fptr(0), bptr(false)),
		tel(base.Add(time.Second), fptr(80), bptr(true)),
		tel(base.Add(2 * time.Second), fptr(90), bptr(true)),
	}
	// Arrival order shuffled; rebuild must sort by event time.
	log[0], log[2] = log[2], log[0]
	store := NewMemoryStore()
	if err := RebuildFrom(ctx, store, log); err != nil {
		t.Fatal(err)
	}
	got, ok, _ := store.Get(ctx, "v")
	if !ok || got.Motion != MotionMoving {
		t.Fatalf("rebuild wrong: %+v ok=%v", got, ok)
	}
}

// Late data must not corrupt hot state (watermark = LastSeen).
func TestLateDataIgnored(t *testing.T) {
	var eng Engine
	base := time.Now()
	cur, _ := eng.Apply(VehicleState{}, tel(base, fptr(80), bptr(true)))
	cur, _ = eng.Apply(cur, tel(base.Add(time.Minute), fptr(90), bptr(true)))
	before := cur
	cur, tr := eng.Apply(cur, tel(base.Add(-time.Hour), fptr(0), bptr(false)))
	if len(tr) != 0 {
		t.Fatalf("late point emitted transitions: %v", tr)
	}
	if cur.Motion != before.Motion || !cur.LastSeen.Equal(before.LastSeen) {
		t.Fatal("late point corrupted hot state")
	}
}
