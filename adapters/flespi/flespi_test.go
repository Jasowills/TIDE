package flespi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// T091 acceptance: MQTT live path normalization + REST discovery, fixture-tested.
func TestFixtureNormalization(t *testing.T) {
	raw, err := os.ReadFile("fixtures/telemetry.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := MessageToTelemetry("acme", raw, time.Now())
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if got.Location.Lat != 48.8566 || got.Location.Lng != 2.3522 {
		t.Fatalf("position: %+v", got.Location)
	}
	if got.SpeedKmh == nil || *got.SpeedKmh != 73.5 {
		t.Fatalf("speed (already km/h): %v", got.SpeedKmh)
	}
	if got.Source.Provider != "flespi" || got.Raw == nil {
		t.Fatalf("source/raw: %+v", got.Source)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("canonical invalid: %v", err)
	}
}

func TestContractAgainstFixtureServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gw/devices" {
			w.Write([]byte(`{"result":[{"id":123,"name":"truck-123"}]}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	ctx := context.Background()
	a := New(srv.URL, "token", "acme")
	if _, err := a.ListVehicles(ctx); err == nil {
		t.Fatal("ListVehicles before Connect must fail")
	}
	if err := a.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	vs, err := a.ListVehicles(ctx)
	if err != nil || len(vs) != 1 || vs[0].ProviderID != "123" {
		t.Fatalf("devices: %v %v", vs, err)
	}
	raw, _ := os.ReadFile("fixtures/telemetry.json")
	got, err := a.HandleRaw(raw, time.Now())
	if err != nil {
		t.Fatalf("HandleRaw: %v", err)
	}
	if got.VehicleID == "" {
		t.Fatal("empty vehicle id")
	}
	_ = a.Disconnect(ctx)
}
