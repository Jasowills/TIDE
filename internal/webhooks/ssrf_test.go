package webhooks

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tide-telematics/tide/schemas/events"
)

// A01 table: every SSRF shape fails closed at validation time — before any
// request exists. Uses IP literals (no external DNS) plus scheme smuggling,
// so the test is hermetic and deterministic.
func TestValidateURLBlocksSSRF(t *testing.T) {
	blocked := []string{
		"http://169.254.169.254/latest/meta-data/", // AWS metadata — the objective's exact case
		"http://169.254.169.254:80/",
		"http://127.0.0.1:6379/",   // loopback (redis)
		"http://localhost:3000/x",  // loopback by name
		"http://10.0.0.5/hook",     // RFC1918
		"http://172.16.9.9/hook",   // RFC1918
		"http://192.168.1.1/hook",  // RFC1918
		"http://[::1]/hook",        // IPv6 loopback
		"http://[fe80::1]/hook",    // link-local
		"file:///etc/passwd",       // scheme smuggling
		"gopher://127.0.0.1:25/",   // protocol smuggling
		"ftp://10.0.0.1/x",         // wrong scheme
		"http://user:pass@93.184.216.34/x", // userinfo smuggling
		"http://metadata.google.internal/", // named metadata endpoint
		"not-a-url",
		"",
	}
	for _, raw := range blocked {
		if err := ValidateURL(raw, false); err == nil {
			t.Fatalf("SSRF allowed: %q", raw)
		} else if !strings.Contains(err.Error(), "ssrf") {
			t.Fatalf("block reason must name ssrf, got %v for %q", err, raw)
		}
	}
}

// A public literal IP validates (no request is made — validation only).
func TestValidateURLAllowsPublic(t *testing.T) {
	if err := ValidateURL("https://93.184.216.34/hook", false); err != nil {
		t.Fatalf("public URL rejected: %v", err)
	}
}

// Firing proof, part 1 (would-be-vulnerable behavior is stopped): with the
// guard at production defaults, dispatch to loopback errors AND the server
// observes zero hits — nothing is ever sent.
func TestDispatchBlockedEmitsZeroRequests(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	d := NewDispatcher() // AllowPrivate=false: production default
	d.MaxRetries = 0
	err := d.Dispatch(srv.URL, "s", evtForSSRF("blocked-1"))
	if err == nil || !strings.Contains(err.Error(), "ssrf") {
		t.Fatalf("expected ssrf block, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("guard fired after %d requests — must be zero", hits.Load())
	}
	if len(d.DeadLetter) != 1 {
		t.Fatalf("blocked dispatch must dead-letter, got %+v", d.DeadLetter)
	}
}

// Firing proof, part 2 (the test harness CAN reach a server): the same
// dispatch with the dev override delivers exactly one request. Together the
// pair proves the assertions fire — part 1 is not vacuous.
func TestDispatchAllowedReachesServer(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	d := NewDispatcher()
	d.AllowPrivate = true
	d.MaxRetries = 0
	if err := d.Dispatch(srv.URL, "s", evtForSSRF("allowed-1")); err != nil {
		t.Fatalf("allowed dispatch failed: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("want exactly 1 request, got %d", hits.Load())
	}
}

// Redirects to internal targets are refused mid-flight, not followed.
func TestRedirectToPrivateRefused(t *testing.T) {
	redirToMeta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/", http.StatusFound)
	}))
	defer redirToMeta.Close()

	d := NewDispatcher()
	d.AllowPrivate = true // outer hop (loopback test server) allowed…
	d.MaxRetries = 0
	if err := d.Dispatch(redirToMeta.URL, "s", evtForSSRF("redir-1")); err == nil {
		t.Fatal("redirect to metadata IP must fail closed")
	}
}

func evtForSSRF(id string) events.Event {
	return events.Event{ID: id, Type: "vehicle.speeding.started", TenantID: "t",
		VehicleID: "v", Timestamp: time.Now(), CorrelationID: "c", SchemaVersion: 1}
}
