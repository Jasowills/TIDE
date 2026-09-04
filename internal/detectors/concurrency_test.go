package detectors

import (
	"sync"
	"testing"
	"time"

	"github.com/tide-telematics/tide/internal/state"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

// Regression: shared Tracker across goroutines (the HTTP server fans Process
// out per request). Before the mutex this was a FATAL concurrent-map crash
// under spike load — run the suite with -race (CI does).
func TestConcurrentDetect(t *testing.T) {
	tr := NewTracker(DefaultConfig())
	base := time.Now()
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				sp := 30 + float64((g+i)%90)
				ign := true
				tel := ctelemetry.Telemetry{
					ID: "x", TenantID: "t", VehicleID: string(rune('a' + g%4)), DeviceID: "d",
					Timestamp: base.Add(time.Duration(i) * time.Second), ReceivedAt: base,
					Location:  ctelemetry.Location{Lat: 1, Lng: 2},
					SpeedKmh:  &sp, Ignition: &ign,
					Raw:       map[string]any{},
					Metadata:  ctelemetry.Metadata{CorrelationID: "c", SchemaVersion: 1},
				}
				_ = tr.Detect(tel, state.VehicleState{Presence: state.PresenceOnline})
			}
		}(g)
	}
	wg.Wait()
}
