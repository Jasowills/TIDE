package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tide-telematics/tide/internal/dedup"
	"github.com/tide-telematics/tide/internal/detectors"
	"github.com/tide-telematics/tide/internal/eventbus"
	"github.com/tide-telematics/tide/internal/geo"
	"github.com/tide-telematics/tide/internal/pipeline"
	"github.com/tide-telematics/tide/internal/state"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
	"github.com/tide-telematics/tide/schemas/events"
)

type idSpyRules struct{ ids []string }

func (s *idSpyRules) Evaluate(_ context.Context, t ctelemetry.Telemetry, _ state.VehicleState, evs []events.Event) []events.Event {
	s.ids = append(s.ids, t.ID)
	return nil
}

func advServer(bus eventbus.Bus, rules pipeline.Rules) *Server {
	p := &pipeline.Pipeline{
		Dedup: dedup.NewMemoryStore(0), States: state.NewMemoryStore(),
		Log: &pipeline.MemoryLog{}, Bus: bus,
		Detectors: detectors.NewTracker(detectors.DefaultConfig()),
		Rules:     rules,
	}
	mem, _ := bus.(*eventbus.MemoryBus)
	return New(Deps{Pipeline: p, States: p.States, Bus: mem,
		Geo: geo.NewTracker(nil), Geofences: NewGeofenceStore(nil)})
}

const advPoint = `{"tenantId":"t","vehicleId":"v","deviceId":"d","timestamp":"2026-09-04T21:00:00Z","location":{"lat":1,"lng":2},"speed":10,"raw":{},"source":{"provider":"x","protocol":"http","deviceId":"d"},"metadata":{"correlationId":"c","schemaVersion":1,"quality":"good"}}`

// ADV-0001: ID-less points leave with server-minted, distinct IDs.
func TestServerAssignsIDs(t *testing.T) {
	spy := &idSpyRules{}
	srv := advServer(&eventbus.MemoryBus{}, spy)
	for i := 0; i < 2; i++ {
		// Distinct timestamps: identical bodies would be true duplicates.
		body := strings.Replace(advPoint, "2026-09-04T21:00:00Z", "2026-09-04T21:00:0"+string(rune('0'+i))+"Z", 1)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/telemetry", strings.NewReader(body)))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("want 202, got %d", rec.Code)
		}
	}
	if len(spy.ids) != 2 {
		t.Fatalf("want 2 processed points, got %d", len(spy.ids))
	}
	if spy.ids[0] == "" || spy.ids[1] == "" {
		t.Fatalf("empty server ID: %q", spy.ids)
	}
	if spy.ids[0] == spy.ids[1] {
		t.Fatal("duplicate server IDs across distinct receipts")
	}
}

// ADV-0002: far-future points are 422, not silent state poison.
func TestFuturePointRejected422(t *testing.T) {
	srv := advServer(&eventbus.MemoryBus{}, nil)
	future := strings.Replace(advPoint, "2026-09-04T21:00:00Z", "2100-01-01T00:00:00Z", 1)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/telemetry", strings.NewReader(future)))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", rec.Code)
	}
}

type failingBus struct{}

func (failingBus) Publish(context.Context, events.Event) error { return errors.New("down") }

// ADV-0005: bus failure → 503 retryable (not silent loss, not generic 500).
func TestBusFailureIs503(t *testing.T) {
	srv := advServer(failingBus{}, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/telemetry", strings.NewReader(advPoint)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d: %s", rec.Code, rec.Body.String())
	}
}
