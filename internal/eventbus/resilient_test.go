package eventbus

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tide-telematics/tide/schemas/events"
)

type stubBus struct {
	failsLeft atomic.Int32
	calls     atomic.Int32
}

func (s *stubBus) Publish(_ context.Context, e events.Event) error {
	s.calls.Add(1)
	if s.failsLeft.Add(-1) >= 0 {
		return errors.New("boom")
	}
	return nil
}

func evt() events.Event {
	return events.Event{ID: "e1", Type: "vehicle.speeding.started", TenantID: "t",
		VehicleID: "v", Timestamp: time.Now(), CorrelationID: "c", SchemaVersion: 1}
}

// ADV-0005: a transient transport failure redials once and the publish lands.
func TestResilientRedialsOnce(t *testing.T) {
	inner := &stubBus{}
	inner.failsLeft.Store(1)
	var dials atomic.Int32
	r := NewResilientBus(func() (Bus, error) {
		dials.Add(1)
		return inner, nil
	}, inner)
	if err := r.Publish(context.Background(), evt()); err != nil {
		t.Fatalf("redialed publish failed: %v", err)
	}
	if dials.Load() != 1 {
		t.Fatalf("want exactly 1 redial, got %d", dials.Load())
	}
	if ts, _ := r.LastError(); !ts.IsZero() {
		t.Fatal("successful publish must not record an error")
	}
}

// Persistent outage: bounded failure, error recorded for health reporting.
func TestResilientGivesUpCleanly(t *testing.T) {
	inner := &stubBus{}
	inner.failsLeft.Store(1000)
	r := NewResilientBus(func() (Bus, error) { return inner, nil }, inner)
	if err := r.Publish(context.Background(), evt()); err == nil {
		t.Fatal("expected error during persistent outage")
	}
	ts, msg := r.LastError()
	if ts.IsZero() || msg == "" {
		t.Fatal("failure must be recorded for health reporting")
	}
}

// SubscribeResilient against a dead endpoint: must not hang, panic, or leak
// goroutines past ctx cancel.
func TestSubscribeResilientDeadEndpoint(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	SubscribeResilient(ctx, "nats://127.0.0.1:14222", "tide.test.>", func([]byte) {})
	go func() {
		<-ctx.Done()
		time.Sleep(500 * time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("resilient subscribe did not unwind after ctx cancel")
	}
}
