package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tide-telematics/tide/internal/detectors"
	"github.com/tide-telematics/tide/internal/eventbus"
	"github.com/tide-telematics/tide/internal/geo"
	"github.com/tide-telematics/tide/internal/pipeline"
)

func simServer() *Server {
	p, _, _ := pipeline.NewTestPipeline()
	p.Detectors = detectors.NewTracker(detectors.DefaultConfig())
	return New(Deps{Pipeline: p, States: p.States, Bus: &eventbus.MemoryBus{},
		Geo: geo.NewTracker(nil), Geofences: NewGeofenceStore(nil)})
}

func postSim(t *testing.T, srv *Server, body string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/simulate", strings.NewReader(body)))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func waitState(t *testing.T, srv *Server, id string, want ...string) map[string]any {
	t.Helper()
	for i := 0; i < 200; i++ {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/simulate/"+id, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		var st map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &st)
		for _, w := range want {
			if st["state"] == w {
				return st
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("run %s never reached %v", id, want)
	return nil
}

// A sim run executes FleetSim through the production pipeline and reports.
func TestSimulateRunCompletes(t *testing.T) {
	srv := simServer()
	code, out := postSim(t, srv, `{"scenario":"speeding","vehicles":3,"tenant":"default"}`)
	if code != http.StatusAccepted {
		t.Fatalf("want 202, got %d (%v)", code, out)
	}
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatal("no run id")
	}
	st := waitState(t, srv, id, "done")
	if st["accepted"].(float64) == 0 {
		t.Fatal("no points accepted")
	}
	if st["events"].(float64) == 0 {
		t.Fatal("speeding scenario produced no events")
	}
}

func TestSimulateValidation(t *testing.T) {
	srv := simServer()
	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"unknown scenario", `{"scenario":"nope","vehicles":2}`, 422},
		{"too many vehicles", `{"scenario":"mixed","vehicles":500}`, 422},
		{"bad fault rate", `{"scenario":"mixed","vehicles":2,"faults":{"duplicateRate":7}}`, 422},
		{"malformed", `{`, 400},
	} {
		if code, _ := postSim(t, srv, tc.body); code != tc.want {
			t.Fatalf("%s: want %d, got %d", tc.name, tc.want, code)
		}
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/simulate/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown run: want 404, got %d", rec.Code)
	}
}

// DELETE on a finished run is a no-op success, never an error.
func TestSimulateDeleteAfterDone(t *testing.T) {
	srv := simServer()
	_, out := postSim(t, srv, `{"scenario":"normal","vehicles":1}`)
	id := out["id"].(string)
	waitState(t, srv, id, "done")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/v1/simulate/"+id, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete finished run: want 200, got %d", rec.Code)
	}
}
