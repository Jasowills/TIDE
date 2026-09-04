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
)

func testDeps() Deps {
	p := &pipeline.Pipeline{
		Dedup: dedup.NewMemoryStore(0), States: state.NewMemoryStore(),
		Log: &pipeline.MemoryLog{}, Bus: &eventbus.MemoryBus{},
		Detectors: detectors.NewTracker(detectors.DefaultConfig()),
	}
	return Deps{Pipeline: p, States: p.States, Bus: &eventbus.MemoryBus{},
		Geo: geo.NewTracker(nil), Geofences: NewGeofenceStore(nil)}
}

// A04: every response carries the baseline headers.
func TestSecurityHeaders(t *testing.T) {
	srv := New(testDeps())
	for _, path := range []string{"/healthz", "/v1/events?tenant=t", "/v1/connections", "/nope"} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		h := rec.Header()
		if h.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s: missing nosniff", path)
		}
		if h.Get("X-Frame-Options") != "DENY" {
			t.Fatalf("%s: missing frame DENY", path)
		}
		if csp := h.Get("Content-Security-Policy"); csp == "" ||
			strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
			t.Fatalf("%s: bad CSP %q", path, csp)
		}
	}
}

// A02: TRACE (and other unregistered methods) are not served.
func TestTraceDisabled(t *testing.T) {
	srv := New(testDeps())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("TRACE", "/healthz", nil))
	if s := rec.Code; s != 403 && s != 405 && s != 501 {
		t.Fatalf("TRACE must not be served, got %d", s)
	}
	if strings.Contains(rec.Body.String(), "TRACE /healthz") {
		t.Fatal("TRACE request echoed")
	}
}

// A06: a burst against ingest gets throttled with 429 + Retry-After.
func TestIngestBurstThrottled(t *testing.T) {
	d := testDeps()
	d.Limiter = NewRateLimiter(5, 60*1000000000)
	srv := New(d)
	body := `{"tenantId":"t","vehicleId":"v","deviceId":"d","timestamp":"2026-01-01T00:00:00Z","location":{"lat":1,"lng":2},"raw":{},"source":{"provider":"x","protocol":"http","deviceId":"d"},"metadata":{"correlationId":"c","schemaVersion":1,"quality":"good"}}`
	var throttled int
	for i := 0; i < 15; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/telemetry", strings.NewReader(body))
		req.RemoteAddr = "10.9.9.9:1234"
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			throttled++
			if rec.Header().Get("Retry-After") == "" {
				t.Fatal("429 without Retry-After")
			}
		}
	}
	if throttled == 0 {
		t.Fatal("burst of 15 against a limit-5 window produced no 429")
	}
}

type failingStates struct{ state.Store }

func (f failingStates) Get(_ context.Context, _ string) (state.VehicleState, bool, error) {
	return state.VehicleState{}, false, errors.New("postgres: dial tcp 10.0.0.5:5432: connect: boom; table events missing")
}

// A10: backend failures fail closed with a generic body — no driver names,
// addresses, or schema details reach the client.
func TestErrorContractLeaksNothing(t *testing.T) {
	d := testDeps()
	d.States = failingStates{d.States}
	srv := New(d)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/vehicles/v/state", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
	body := rec.Body.String()
	if body != "internal error\n" {
		t.Fatalf("body must be exactly the generic contract, got %q", body)
	}
	for _, leak := range []string{"postgres", "dial", "10.0.0.5", "events", "table"} {
		if strings.Contains(body, leak) {
			t.Fatalf("leaked %q in error body", leak)
		}
	}
}

// A05: SQL-flavored input is data, never a query — it validates or 4xxs,
// never 500s, and the error text carries no DB vocabulary.
func TestInjectionInputFailsClosed(t *testing.T) {
	srv := New(testDeps())
	payloads := []string{
		`{"tenantId":"' OR '1'='1","vehicleId":"v","deviceId":"d","timestamp":"2026-01-01T00:00:00Z","location":{"lat":1,"lng":2},"raw":{},"source":{"provider":"x","protocol":"http","deviceId":"d"},"metadata":{"correlationId":"c","schemaVersion":1,"quality":"good"}}`,
		`{"tenantId":"t","vehicleId":"1 UNION SELECT null--","deviceId":"d","timestamp":"2026-01-01T00:00:00Z","location":{"lat":1,"lng":2},"raw":{},"source":{"provider":"x","protocol":"http","deviceId":"d"},"metadata":{"correlationId":"c","schemaVersion":1,"quality":"good"}}`,
	}
	for _, body := range payloads {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/telemetry", strings.NewReader(body)))
		if rec.Code == http.StatusInternalServerError {
			t.Fatalf("injection-shaped input caused 500: %s", body[:60])
		}
		text := rec.Body.String()
		for _, leak := range []string{"SQL", "syntax error", "pq:", "pg:"} {
			if strings.Contains(text, leak) {
				t.Fatalf("leaked %q for input %s", leak, body[:40])
			}
		}
	}
}
