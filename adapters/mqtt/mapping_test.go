package mqtt

import (
	"testing"
	"time"
)

const sampleMapping = `
broker: tcp://localhost:1883
topics: ["fleet/+/telemetry"]
defaults: {tenant: t1, provider: mytracker}
fields:
  lat: gps.lat
  lng: gps.lon
  speed: gps.speed
  ignition: ignition
  timestamp: ts
  sequence: seq
speedUnit: mph
topicVehicleIndex: 1
`

func TestMapPayloadZeroGoCode(t *testing.T) {
	cfg, err := LoadMapping([]byte(sampleMapping))
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"gps":{"lat":52.5,"lon":13.4,"speed":62},"ignition":true,"ts":"2026-01-01T00:00:00Z","seq":7}`
	got, err := MapPayload(cfg, "fleet/v9/telemetry", []byte(raw), time.Now())
	if err != nil {
		t.Fatalf("MapPayload: %v", err)
	}
	if got.VehicleID != "v9" || got.TenantID != "t1" {
		t.Fatalf("identity: %+v", got)
	}
	want := 62 * 1.60934 // mph → km/h at adapter boundary
	if got.SpeedKmh == nil || *got.SpeedKmh-want > 1e-9 {
		t.Fatalf("speed conversion: got %v want %v", got.SpeedKmh, want)
	}
	if got.Ignition == nil || !*got.Ignition {
		t.Fatal("ignition not mapped")
	}
	if got.Metadata.Sequence == nil || *got.Metadata.Sequence != 7 {
		t.Fatal("sequence not mapped")
	}
	if got.Raw == nil || got.Raw["gps"] == nil {
		t.Fatal("raw payload must be preserved")
	}
}

func TestMapPayloadBadConfig(t *testing.T) {
	if _, err := LoadMapping([]byte("speedUnit: furlongs")); err == nil {
		t.Fatal("expected error")
	}
	if _, err := LoadMapping([]byte("broker: x")); err == nil {
		t.Fatal("expected error for missing topics")
	}
}
