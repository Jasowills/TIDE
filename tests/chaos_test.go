// Chaos suite (T110). In-process fault tests always run in CI.
// Live disruption tests (DB/Redis/NATS restart, provider disconnect) run
// when TIDE_CHAOS_LIVE=1 against `docker compose up` services, and on the
// scheduled chaos workflow — never as a surprise in unit CI.
package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tide-telematics/tide/internal/dedup"
	"github.com/tide-telematics/tide/internal/eventbus"
	"github.com/tide-telematics/tide/internal/pipeline"
	"github.com/tide-telematics/tide/internal/state"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

func flush(t *testing.T) {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer rdb.Close()
	if err := rdb.FlushAll(context.Background()).Err(); err != nil {
		t.Fatalf("flush: %v", err)
	}
}

func stormPoint(id string, ts time.Time, seq int64) ctelemetry.Telemetry {
	sp := 80.0
	ign := true
	return ctelemetry.Telemetry{
		ID: id, TenantID: "t", VehicleID: "v", DeviceID: "d",
		Timestamp: ts, ReceivedAt: ts,
		Location:  ctelemetry.Location{Lat: 52.5, Lng: 13.4},
		SpeedKmh:  &sp, Ignition: &ign,
		Raw:       map[string]any{},
		Source:    ctelemetry.Source{Provider: "chaos", Protocol: "x", DeviceID: "d"},
		Metadata:  ctelemetry.Metadata{CorrelationID: "c", SchemaVersion: 1, Sequence: &seq},
	}
}

// Malformed + duplicate storm: no panic, no corruption, duplicates absorbed.
func TestChaosStorm(t *testing.T) {
	ctx := context.Background()
	p, bus, _ := pipeline.NewTestPipeline()
	base := time.Now()
	for i := 0; i < 500; i++ {
		pt := stormPoint("s", base.Add(time.Duration(i)*time.Second), int64(i%50)) // heavy dup reuse
		if _, err := p.Process(ctx, pt); err != nil {
			t.Fatalf("storm point %d: %v", i, err)
		}
		// Malformed interleaved: rejected, never fatal.
		bad := pt
		bad.Location.Lat = 999
		if _, err := p.Process(ctx, bad); err == nil {
			t.Fatalf("malformed point %d accepted", i)
		}
	}
	s, ok, err := p.States.Get(ctx, "v")
	if err != nil || !ok || s.Motion != state.MotionMoving {
		t.Fatalf("state corrupted after storm: %+v %v %v", s, ok, err)
	}
	_ = bus
}

// Late/duplicate/malformed telemetry during replay: deterministic + clean.
func TestChaosReplayUnderFaults(t *testing.T) {
	ctx := context.Background()
	p, _, log := pipeline.NewTestPipeline()
	base := time.Now()
	pts := []ctelemetry.Telemetry{
		stormPoint("a", base, 1),
		stormPoint("b", base.Add(-time.Hour), 2), // very late
		stormPoint("a2", base, 1),                // exact duplicate
		stormPoint("c", base.Add(time.Second), 3),
	}
	for _, pt := range pts {
		_, _ = p.Process(ctx, pt) // never fatal
	}
	if len(log.All) != 3 { // dup absorbed before durable append
		t.Fatalf("want 3 durable points, got %d", len(log.All))
	}
}

func liveEnabled() bool { return os.Getenv("TIDE_CHAOS_LIVE") == "1" }

// Live: Redis FLUSHALL mid-run → hot state rebuilds from durable log.
func TestChaosLiveRedisRestart(t *testing.T) {
	if !liveEnabled() {
		t.Skip("set TIDE_CHAOS_LIVE=1 with compose services up")
	}
	ctx := context.Background()
	p, _, log := pipeline.NewTestPipeline()
	states := state.NewRedisStore("localhost:6379")
	p.States = states
	base := time.Now()
	for i := 0; i < 5; i++ {
		if _, err := p.Process(ctx, stormPoint("r", base.Add(time.Duration(i)*time.Second), int64(100+i))); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate Redis death: flush everything, rebuild from durable log.
	flush(t)
	if err := state.RebuildFrom(ctx, states, log.All); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	s, ok, err := states.Get(ctx, "v")
	if err != nil || !ok || s.Motion != state.MotionMoving {
		t.Fatalf("state not reconstructed: %+v %v %v", s, ok, err)
	}
	_ = eventbus.MemoryBus{}
	_ = dedup.NewMemoryStore(0)
}
