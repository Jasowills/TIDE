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

type flappingStore struct {
	fail bool
	got  []ctelemetry.Telemetry
}

func (f *flappingStore) AppendTelemetry(_ context.Context, t ctelemetry.Telemetry) error {
	if f.fail {
		return errors.New("postgres down")
	}
	f.got = append(f.got, t)
	return nil
}

// §2.10: a dead Postgres must degrade to the memory buffer, never fail ingestion.
func TestPostgresDownDoesNotKillIngestion(t *testing.T) {
	ctx := context.Background()
	bus := &eventbus.MemoryBus{}
	log := &pgLog{pg: failingStore{}}
	p := &pipeline.Pipeline{
		Dedup: dedup.NewMemoryStore(0), States: state.NewMemoryStore(),
		Log: log, Bus: bus, Detectors: detectors.NewTracker(detectors.DefaultConfig()),
	}
	sp := 60.0
	ign := true
	if _, _, err := p.Process(ctx, ctelemetry.Telemetry{
		ID: "x", TenantID: "t", VehicleID: "v", DeviceID: "d",
		Timestamp: time.Now(), ReceivedAt: time.Now(),
		Location:  ctelemetry.Location{Lat: 1, Lng: 2},
		SpeedKmh:  &sp, Ignition: &ign, Raw: map[string]any{},
		Source:    ctelemetry.Source{Provider: "x", Protocol: "x", DeviceID: "d"},
		Metadata:  ctelemetry.Metadata{CorrelationID: "c", SchemaVersion: 1},
	}); err != nil {
		t.Fatalf("ingestion died with postgres: %v", err)
	}
	if log.Pending() != 1 {
		t.Fatalf("point not buffered: %d", log.Pending())
	}
}

// ADV-0004: what was accepted during the outage reaches PG after recovery.
func TestDrainOnRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &flappingStore{fail: true}
	log := &pgLog{pg: store}
	log.StartDrain(ctx)
	sp := 60.0
	for _, id := range []string{"d1", "d2", "d3"} {
		if err := log.Append(ctx, ctelemetry.Telemetry{
			ID: id, TenantID: "t", VehicleID: "v", DeviceID: "d",
			Timestamp: time.Now(), ReceivedAt: time.Now(),
			Location:  ctelemetry.Location{Lat: 1, Lng: 2},
			SpeedKmh:  &sp, Raw: map[string]any{},
			Source:    ctelemetry.Source{Provider: "x", Protocol: "x", DeviceID: "d"},
			Metadata:  ctelemetry.Metadata{CorrelationID: "c", SchemaVersion: 1},
		}); err != nil {
			t.Fatalf("buffered append failed: %v", err)
		}
	}
	if log.Pending() != 3 {
		t.Fatalf("want 3 pending, got %d", log.Pending())
	}
	store.fail = false // PG recovers
	deadline := time.Now().Add(15 * time.Second)
	for log.Pending() > 0 && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	if log.Pending() != 0 {
		t.Fatalf("drain stalled with %d pending", log.Pending())
	}
	if len(store.got) != 3 {
		t.Fatalf("want 3 drained points, got %d", len(store.got))
	}
	// Order preserved (drain is FIFO).
	for i, id := range []string{"d1", "d2", "d3"} {
		if store.got[i].ID != id {
			t.Fatalf("drain order wrong: %+v", store.got)
		}
	}
}
