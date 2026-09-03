package events

import (
	"testing"
	"time"
)

func TestValidateTransitionSuffix(t *testing.T) {
	e := Event{ID: "1", Type: "vehicle.speeding.started", TenantID: "t", VehicleID: "v",
		Timestamp: time.Now(), CorrelationID: "c", SchemaVersion: CurrentSchemaVersion}
	if err := e.Validate(); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}
	e.Type = "vehicle.speeding" // raw-packet-as-event: forbidden
	if err := e.Validate(); err == nil {
		t.Fatal("expected rejection of suffix-less type")
	}
}

func TestDeterministicIDStable(t *testing.T) {
	ts := time.Now()
	a := DeterministicID("vehicle.speeding.started", "v", ts, "rule")
	b := DeterministicID("vehicle.speeding.started", "v", ts, "rule")
	if a != b {
		t.Fatal("deterministic ids must match")
	}
}
