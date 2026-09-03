package detectors

import (
	"testing"
	"time"

	"github.com/tide-telematics/tide/internal/state"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

func vehState(ts time.Time) state.VehicleState {
	return state.VehicleState{Motion: state.MotionMoving, Presence: state.PresenceOnline, LastSeen: ts}
}

func vehStateMoving(ts time.Time, moving bool) state.VehicleState {
	m := state.MotionStopped
	if moving {
		m = state.MotionMoving
	}
	return state.VehicleState{Motion: m, Presence: state.PresenceOnline, LastSeen: ts}
}

func tel(ts time.Time, speed float64, ign bool) ctelemetry.Telemetry {
	return ctelemetry.Telemetry{
		ID: "x", TenantID: "t", VehicleID: "v", DeviceID: "d",
		Timestamp: ts, ReceivedAt: ts,
		Location:  ctelemetry.Location{Lat: 52.5, Lng: 13.4},
		SpeedKmh:  &speed, Ignition: &ign,
		Raw:       map[string]any{},
		Source:    ctelemetry.Source{Provider: "sim"},
		Metadata:  ctelemetry.Metadata{CorrelationID: "c", SchemaVersion: 1},
	}
}

func TestSpeedingStartedContinuedEnded(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SpeedLimitKmh = 50
	cfg.SpeedForSecs = 5
	cfg.SpeedContinuedEvery = 30 * time.Second
	tr := NewTracker(cfg)
	base := time.Now()
	var types []string
	// 10s over limit → started once; keep speeding 70s → continued; slow → ended.
	for i := 0; i < 80; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		evs := tr.Detect(tel(ts, 90, true), vehState(ts))
		for _, e := range evs {
			if len(e.Type) >= 17 && e.Type[:17] == "vehicle.speeding." {
				types = append(types, e.Type)
			}
		}
	}
	evs := tr.Detect(tel(base.Add(81*time.Second), 10, true), vehState(base))
	for _, e := range evs {
		types = append(types, e.Type)
	}
	started, continued, ended := 0, 0, 0
	for _, x := range types {
		switch x {
		case "vehicle.speeding.started":
			started++
		case "vehicle.speeding.continued":
			continued++
		case "vehicle.speeding.ended":
			ended++
		}
	}
	if started != 1 {
		t.Fatalf("want exactly 1 started, got %d (%v)", started, types)
	}
	if continued < 1 {
		t.Fatalf("want ≥1 continued, got %v", types)
	}
	if ended != 1 {
		t.Fatalf("want exactly 1 ended, got %v", types)
	}
}

func TestTripStartEnd(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TripEndSecs = 10
	tr := NewTracker(cfg)
	base := time.Now()
	var types []string
	pts := []struct {
		off   int
		speed float64
		ign   bool
	}{{0, 0, false}, {1, 60, true}, {2, 65, true}, {3, 0, false}}
	for _, p := range pts {
		ts := base.Add(time.Duration(p.off) * time.Second)
		for _, e := range tr.Detect(tel(ts, p.speed, p.ign), vehStateMoving(ts, p.speed > 5)) {
			types = append(types, e.Type)
		}
	}
	found := map[string]bool{}
	for _, x := range types {
		found[x] = true
	}
	if !found["vehicle.trip.started"] || !found["vehicle.trip.ended"] {
		t.Fatalf("trip start/end missing: %v", types)
	}
}
