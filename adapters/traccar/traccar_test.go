package traccar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// T090 acceptance: contract-tested against a recorded fixture, version-pinned.
func TestFixtureNormalization(t *testing.T) {
	raw, err := os.ReadFile("fixtures/positions.json")
	if err != nil {
		t.Fatal(err)
	}
	var positions []Position
	if err := json.Unmarshal(raw, &positions); err != nil {
		t.Fatal(err)
	}
	got, err := PositionToTelemetry("acme", positions[0], time.Now())
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	// 48.6 knots → km/h at the adapter boundary.
	want := 48.6 * 1.852
	if got.SpeedKmh == nil || *got.SpeedKmh-want > 1e-9 {
		t.Fatalf("knots conversion: got %v want %v", got.SpeedKmh, want)
	}
	if got.Ignition == nil || !*got.Ignition {
		t.Fatal("ignition not carried")
	}
	if got.Source.Provider != "traccar" || got.Raw == nil {
		t.Fatalf("source/raw: %+v", got.Source)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("canonical invalid: %v", err)
	}
}

func TestContractAgainstFixtureServer(t *testing.T) {
	positions, _ := os.ReadFile("fixtures/positions.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/server":
			w.Write([]byte(`{"version":"6.5.0"}`))
		case "/api/positions":
			w.Write(positions)
		case "/api/devices":
			w.Write([]byte(`[{"id":7,"name":"van-7"}]`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	a := New(srv.URL, "admin", "admin", "acme")

	// Forced failure first: listing while disconnected must error.
	if _, err := a.ListVehicles(ctx); err == nil {
		t.Fatal("ListVehicles before Connect must fail")
	}
	if err := a.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if st := a.Health(ctx); st.State != "HEALTHY" {
		t.Fatalf("health: %+v", st)
	}
	vs, err := a.ListVehicles(ctx)
	if err != nil || len(vs) != 1 || vs[0].ProviderID != "7" {
		t.Fatalf("devices: %v %v", vs, err)
	}
	v, err := a.GetVehicle(ctx, "7")
	if err != nil || v.Name != "van-7" {
		t.Fatalf("get: %v %v", v, err)
	}
	caps := a.Capabilities()
	if !caps.LiveTelemetry || !caps.DeviceList {
		t.Fatalf("capabilities: %+v", caps)
	}
	_ = a.Disconnect(ctx)
}
