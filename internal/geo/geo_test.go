package geo

import (
	"context"
	"testing"
	"time"

	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

var square = Geofence{ID: "g1", TenantID: "t", Name: "depot", Polygon: []Point{
	{Lat: 0, Lng: 0}, {Lat: 0, Lng: 10}, {Lat: 10, Lng: 10}, {Lat: 10, Lng: 0},
}}

func telAt(lat, lng float64, ts time.Time) ctelemetry.Telemetry {
	return ctelemetry.Telemetry{
		ID: "x", TenantID: "t", VehicleID: "v", DeviceID: "d",
		Timestamp: ts, ReceivedAt: ts,
		Location:  ctelemetry.Location{Lat: lat, Lng: lng},
		Raw:       map[string]any{},
		Metadata:  ctelemetry.Metadata{CorrelationID: "c", SchemaVersion: 1},
	}
}

func TestContains(t *testing.T) {
	if !square.Contains(Point{5, 5}) {
		t.Fatal("center should be inside")
	}
	if square.Contains(Point{20, 20}) {
		t.Fatal("far point should be outside")
	}
}

// QA §3.1.5: enter-then-exit → exactly one entered + one exited, never repeated.
func TestEnterExitExactlyOnce(t *testing.T) {
	ctx := context.Background()
	tr := NewTracker([]Geofence{square})
	base := time.Now()
	entered, exited := 0, 0
	// Outside → dwell inside (many points) → outside. Repeated insides must
	// not re-emit entered.
	pts := []Point{{-5, -5}, {5, 5}, {5, 6}, {5, 7}, {6, 6}, {20, 20}, {21, 21}}
	for i, p := range pts {
		for _, e := range tr.Evaluate(ctx, telAt(p.Lat, p.Lng, base.Add(time.Duration(i)*time.Minute))) {
			switch e.Type {
			case "vehicle.geofence.entered":
				entered++
			case "vehicle.geofence.exited":
				exited++
			}
			if err := e.Validate(); err != nil {
				t.Fatalf("invalid event: %v", err)
			}
		}
	}
	if entered != 1 || exited != 1 {
		t.Fatalf("want 1 entered + 1 exited, got %d + %d", entered, exited)
	}
}
