// Benchmarks (T111). Every published number ships with hardware spec, config
// and this reproduction script — a number without a script never enters docs.
// Owner: Jason (maintainer). Run: go test -bench=. -benchtime=10000x ./benchmarks/
package benchmarks

import (
	"context"
	"testing"
	"time"

	"github.com/tide-telematics/tide/internal/pipeline"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

func benchPipeline(b *testing.B, vehicles int) {
	p, _, _ := pipeline.NewTestPipeline()
	ctx := context.Background()
	base := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := i % vehicles
		sp := 60.0
		ign := true
		seq := int64(i)
		_, err := p.Process(ctx, ctelemetry.Telemetry{
			ID: "b", TenantID: "t", VehicleID: string(rune('a'+v)) + "-v",
			DeviceID: "d", Timestamp: base.Add(time.Duration(i) * time.Second), ReceivedAt: base,
			Location:  ctelemetry.Location{Lat: 52.5, Lng: 13.4},
			SpeedKmh:  &sp, Ignition: &ign,
			Raw:       map[string]any{},
			Source:    ctelemetry.Source{Provider: "bench", Protocol: "x", DeviceID: "d"},
			Metadata:  ctelemetry.Metadata{CorrelationID: "b", SchemaVersion: 1, Sequence: &seq},
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIngestSingleVehicle(b *testing.B) { benchPipeline(b, 1) }
func BenchmarkIngest100Vehicles(b *testing.B)   { benchPipeline(b, 100) }
