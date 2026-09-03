package telemetry

import (
	"testing"
	"time"
)

func good() Telemetry {
	return Telemetry{
		ID: "t1", TenantID: "tn", VehicleID: "v", DeviceID: "d",
		Timestamp: time.Now(), ReceivedAt: time.Now(),
		Location:  Location{Lat: 52.5, Lng: 13.4},
		Raw:       map[string]any{},
		Source:    Source{Provider: "sim", Protocol: "mqtt", DeviceID: "d"},
		Metadata:  Metadata{CorrelationID: "c", SchemaVersion: CurrentSchemaVersion, Quality: "good"},
	}
}

func TestValidateAcceptsGood(t *testing.T) {
	if err := good().Validate(); err != nil {
		t.Fatalf("good telemetry rejected: %v", err)
	}
}

func TestValidateRejectsMalformed(t *testing.T) {
	cases := map[string]func(*Telemetry){
		"no tenant":   func(x *Telemetry) { x.TenantID = "" },
		"bad lat":     func(x *Telemetry) { x.Location.Lat = 91 },
		"neg speed":   func(x *Telemetry) { s := -1.0; x.SpeedKmh = &s },
		"bad version": func(x *Telemetry) { x.Metadata.SchemaVersion = 99 },
		"nil raw":     func(x *Telemetry) { x.Raw = nil },
	}
	for name, mut := range cases {
		x := good()
		mut(&x)
		if err := x.Validate(); err == nil {
			t.Fatalf("%s: expected rejection, got nil", name)
		}
	}
}

func TestDedupKeyPrefersSequence(t *testing.T) {
	a, b := good(), good()
	seq := int64(42)
	a.Metadata.Sequence = &seq
	b.Metadata.Sequence = &seq
	b.Timestamp = b.Timestamp.Add(time.Hour) // same key despite different time
	if a.DedupKey() != b.DedupKey() {
		t.Fatal("sequence-based keys should match")
	}
	c := good()
	if a.DedupKey() == c.DedupKey() {
		t.Fatal("different payloads should (almost surely) differ")
	}
}
