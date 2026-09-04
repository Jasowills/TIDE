package boot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tide-telematics/tide/internal/dedup"
	"github.com/tide-telematics/tide/internal/detectors"
	"github.com/tide-telematics/tide/internal/eventbus"
	"github.com/tide-telematics/tide/internal/pipeline"
	"github.com/tide-telematics/tide/internal/state"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

type failingStore struct{}

func (failingStore) AppendTelemetry(_ context.Context, _ ctelemetry.Telemetry) error {
	return errors.New("postgres down")
}

// §2.10: a dead Postgres must degrade to the memory buffer, never fail ingestion.
func TestPostgresDownDoesNotKillIngestion(t *testing.T) {
	ctx := context.Background()
	bus := &eventbus.MemoryBus{}
	log := &pgLog{pg: failingStore{}, mem: &pipeline.MemoryLog{}}
	p := &pipeline.Pipeline{
		Dedup: dedup.NewMemoryStore(0), States: state.NewMemoryStore(),
		Log: log, Bus: bus, Detectors: detectors.NewTracker(detectors.DefaultConfig()),
	}
	sp := 60.0
	ign := true
	if _, err := p.Process(ctx, ctelemetry.Telemetry{
		ID: "x", TenantID: "t", VehicleID: "v", DeviceID: "d",
		Timestamp: time.Now(), ReceivedAt: time.Now(),
		Location:  ctelemetry.Location{Lat: 1, Lng: 2},
		SpeedKmh:  &sp, Ignition: &ign, Raw: map[string]any{},
		Source:    ctelemetry.Source{Provider: "x", Protocol: "x", DeviceID: "d"},
		Metadata:  ctelemetry.Metadata{CorrelationID: "c", SchemaVersion: 1},
	}); err != nil {
		t.Fatalf("ingestion died with postgres: %v", err)
	}
	if len(log.mem.All) != 1 {
		t.Fatalf("point not buffered: %d", len(log.mem.All))
	}
}
