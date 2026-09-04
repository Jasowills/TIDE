package webhooks

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tide-telematics/tide/schemas/events"
)

func evt(id string) events.Event {
	return events.Event{ID: id, Type: "vehicle.speeding.started", TenantID: "t",
		VehicleID: "v", Timestamp: time.Now(), CorrelationID: "c", SchemaVersion: 1}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	body := []byte(`{"a":1}`)
	sig, ts := Sign("s3cr3t", "e1", time.Now().UTC(), body)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(HeaderEventID, "e1")
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderSignature, sig)
	if err := Verify("s3cr3t", req, body); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := Verify("wrong", req, body); err == nil {
		t.Fatal("wrong secret must fail")
	}
	// Tampered body fails.
	if err := Verify("s3cr3t", req, []byte(`{"a":2}`)); err == nil {
		t.Fatal("tampered body must fail")
	}
}

func TestRetryThenDeliver(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		if calls.Add(1) < 3 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	d := NewDispatcher()
	d.AllowPrivate = true // httptest serves loopback
	d.BaseDelay = time.Millisecond
	if err := d.Dispatch(srv.URL, "s", evt("e1")); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(d.Delivered) != 1 || calls.Load() != 3 {
		t.Fatalf("want 3 attempts then delivered, got %d calls", calls.Load())
	}
}

// T063: failing endpoint dead-letters after N retries without blocking ingestion.
func TestDeadLetter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	d := NewDispatcher()
	d.AllowPrivate = true // httptest serves loopback
	d.MaxRetries = 2
	d.BaseDelay = time.Millisecond
	start := time.Now()
	if err := d.Dispatch(srv.URL, "s", evt("e9")); err == nil {
		t.Fatal("expected dead-letter error")
	}
	if el := time.Since(start); el > 10*time.Second {
		t.Fatalf("dispatch blocked ingestion: %v", el)
	}
	if len(d.DeadLetter) != 1 || d.DeadLetter[0].EventID != "e9" {
		t.Fatalf("dead letter missing: %+v", d.DeadLetter)
	}
}
