// Package ingest implements HTTP ingestion (T020): POST /v1/telemetry + :batch.
// Valid payloads become canonical telemetry with received_at set server-side.
package ingest

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

// NewID mints a server-side telemetry ID (crypto-random hex). Entry points
// must assign IDs to ID-less points: without this, every ID-less point shares
// id="" and collides on the telemetry primary key (ADV-0001).
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

type Handler struct {
	Process func(r *http.Request, t ctelemetry.Telemetry) error
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var batch []json.RawMessage
	dec := json.NewDecoder(r.Body)
	var single json.RawMessage
	if err := dec.Decode(&single); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	batch = append(batch, single)
	for dec.More() {
		var extra json.RawMessage
		if err := dec.Decode(&extra); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		batch = append(batch, extra)
	}
	// Also accept {"batch":[...]} envelope.
	var env struct {
		Batch []json.RawMessage `json:"batch"`
	}
	if len(batch) == 1 {
		if err := json.Unmarshal(batch[0], &env); err == nil && env.Batch != nil {
			batch = env.Batch
		}
	}
	received := time.Now().UTC()
	accepted := 0
	for _, raw := range batch {
		var t ctelemetry.Telemetry
		if err := json.Unmarshal(raw, &t); err != nil {
			http.Error(w, "invalid telemetry: "+err.Error(), http.StatusBadRequest)
			return
		}
		if t.ID == "" {
			t.ID = NewID()
		}
		t.ReceivedAt = received // server-side, always (T020)
		if err := t.Validate(); err != nil {
			http.Error(w, "invalid telemetry: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if err := h.Process(r, t); err != nil {
			http.Error(w, "processing failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		accepted++
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"accepted": accepted})
}
