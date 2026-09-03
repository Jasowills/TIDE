package ingest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

func TestSingleAccepted(t *testing.T) {
	var got []ctelemetry.Telemetry
	h := Handler{Process: func(r *http.Request, x ctelemetry.Telemetry) error {
		got = append(got, x)
		return nil
	}}
	body := `{"tenantId":"t","vehicleId":"v","deviceId":"d","timestamp":"2026-01-01T00:00:00Z","location":{"lat":1,"lng":2},"raw":{},"source":{"provider":"x","protocol":"http","deviceId":"d"},"metadata":{"correlationId":"c","schemaVersion":1,"quality":"good"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/telemetry", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(got) != 1 || got[0].ReceivedAt.IsZero() || got[0].ID == "" {
		t.Fatalf("server-side id/received_at not set: %+v", got)
	}
}

func TestBatchEnvelope(t *testing.T) {
	n := 0
	h := Handler{Process: func(r *http.Request, x ctelemetry.Telemetry) error { n++; return nil }}
	item := `{"tenantId":"t","vehicleId":"v","deviceId":"d","timestamp":"2026-01-01T00:00:00Z","location":{"lat":1,"lng":2},"raw":{},"source":{"provider":"x","protocol":"http","deviceId":"d"},"metadata":{"correlationId":"c","schemaVersion":1,"quality":"good"}}`
	body := `{"batch":[` + item + `,` + item + `]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/telemetry:batch", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted || n != 2 {
		t.Fatalf("want 202 x2, got %d n=%d", rec.Code, n)
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["accepted"].(float64) != 2 {
		t.Fatalf("bad accepted count: %v", out)
	}
}

func TestMalformedRejected(t *testing.T) {
	h := Handler{Process: func(r *http.Request, x ctelemetry.Telemetry) error { return nil }}
	req := httptest.NewRequest(http.MethodPost, "/v1/telemetry", strings.NewReader(`{"tenantId":"t"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 422 {
		t.Fatalf("want 422, got %d", rec.Code)
	}
}
