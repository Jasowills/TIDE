// Package api is the tide-api HTTP surface: REST ingestion + queries and a
// WebSocket event stream. Handlers are thin: all logic lives in
// internal/pipeline (the same path replay uses).
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tide-telematics/tide/adapters"
	"github.com/tide-telematics/tide/internal/eventbus"
	"github.com/tide-telematics/tide/internal/geo"
	"github.com/tide-telematics/tide/internal/pipeline"
	"github.com/tide-telematics/tide/internal/rules"
	"github.com/tide-telematics/tide/internal/state"
	"github.com/tide-telematics/tide/internal/store"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
	"github.com/tide-telematics/tide/schemas/events"
	"go.opentelemetry.io/otel"
)

type Deps struct {
	Pipeline  *pipeline.Pipeline
	States    state.Store
	Bus       *eventbus.MemoryBus // nil when NATS backs prod; queries then use PG
	PG        *store.PG           // nil when postgres unreachable (dev without docker)
	Geo       *geo.Tracker
	Geofences *GeofenceStore
	Rules     *rules.Engine
	Registry  *AdapterRegistry
	Limiter   *RateLimiter // nil → New() installs the default 600/min budget
}

// AdapterRegistry tracks adapter health heartbeats (engine publishes on
// tide.heartbeat; console Connections screen reads here). Entries expire
// after 90s without a heartbeat — stale adapters read as unknown, never
// silently healthy.
type AdapterRegistry struct {
	mu      sync.Mutex
	entries map[string]registryEntry
}

type registryEntry struct {
	Status adapters.HealthStatus
	At     time.Time
}

func NewAdapterRegistry() *AdapterRegistry {
	return &AdapterRegistry{entries: map[string]registryEntry{}}
}

func (r *AdapterRegistry) Set(name string, s adapters.HealthStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[name] = registryEntry{Status: s, At: time.Now()}
}

func (r *AdapterRegistry) List() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []map[string]any
	for name, e := range r.entries {
		state := string(e.Status.State)
		if time.Since(e.At) > 90*time.Second {
			state = "STALE"
		}
		out = append(out, map[string]any{
			"name": name, "state": state, "message": e.Status.Message,
			"deviceCount": e.Status.DeviceCount, "msgPerSec": e.Status.MsgPerSec,
			"lastSeen": e.At.UTC().Format(time.RFC3339),
		})
	}
	return out
}

// GeofenceStore is the console-facing geofence registry (memory + PG durable).
type GeofenceStore struct {
	mu     sync.Mutex
	fences []geo.Geofence
	pg     *store.PG
}

func NewGeofenceStore(pg *store.PG) *GeofenceStore { return &GeofenceStore{pg: pg} }

func (g *GeofenceStore) Add(ctx context.Context, f geo.Geofence) error {
	if f.ID == "" || f.TenantID == "" || len(f.Polygon) < 3 {
		return fmt.Errorf("geofence needs id, tenantId, 3+ points")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.fences = append(g.fences, f)
	if g.pg != nil {
		ring := make([][2]float64, len(f.Polygon))
		for i, p := range f.Polygon {
			ring[i] = [2]float64{p.Lng, p.Lat}
		}
		return g.pg.AddGeofence(ctx, f.TenantID, f.ID, f.Name, ring)
	}
	return nil
}

func (g *GeofenceStore) List() []geo.Geofence {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]geo.Geofence{}, g.fences...)
}

// Hub broadcasts events to WebSocket subscribers (console live map/stream).
type Hub struct {
	mu    sync.Mutex
	conns map[*websocket.Conn]bool
}

func (h *Hub) Add(c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns == nil {
		h.conns = map[*websocket.Conn]bool{}
	}
	h.conns[c] = true
}

func (h *Hub) Remove(c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, c)
}

func (h *Hub) Broadcast(e events.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.conns {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.WriteJSON(e); err != nil {
			delete(h.conns, c)
			_ = c.Close()
		}
	}
}

type Server struct {
	deps    Deps
	hub     *Hub
	mux     *http.ServeMux
	handler http.Handler
}

func New(d Deps) *Server {
	s := &Server{deps: d, hub: &Hub{}, mux: http.NewServeMux()}
	if s.deps.Registry == nil {
		s.deps.Registry = NewAdapterRegistry()
	}
	if s.deps.Limiter == nil {
		// Generous production default: abuse throttled, normal use untouched.
		s.deps.Limiter = NewRateLimiter(600, time.Minute)
	}
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, span := otel.Tracer("tide/api").Start(r.Context(), "healthz")
		defer span.End()
		_, _ = w.Write([]byte("ok"))
	})
	s.mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ready"))
	})
	s.mux.HandleFunc("/v1/telemetry", s.deps.Limiter.middleware(http.HandlerFunc(s.handleIngest)).ServeHTTP)
	s.mux.HandleFunc("/v1/telemetry:batch", s.deps.Limiter.middleware(http.HandlerFunc(s.handleIngest)).ServeHTTP)
	s.mux.HandleFunc("/v1/events", s.handleEvents)
	s.mux.HandleFunc("/v1/vehicles/", s.handleVehicle)
	s.mux.HandleFunc("/v1/geofences", s.handleGeofences)
	s.mux.HandleFunc("/v1/rules/triggers", s.handleTriggers)
	s.mux.HandleFunc("/v1/connections", s.handleConnections)
	s.mux.HandleFunc("/v1/stream", s.handleStream)
	s.handler = securityHeaders(denyTrace(s.mux))
	return s
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tr := otel.Tracer("tide/api")
	ctx, span := tr.Start(r.Context(), "ingest")
	defer span.End()
	var batch []json.RawMessage
	var single json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&single); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	batch = append(batch, single)
	var env struct {
		Batch []json.RawMessage `json:"batch"`
	}
	if len(batch) == 1 {
		if err := json.Unmarshal(batch[0], &env); err == nil && env.Batch != nil {
			batch = env.Batch
		}
	}
	received := time.Now().UTC()
	accepted, emitted := 0, 0
	for _, raw := range batch {
		var t ctelemetry.Telemetry
		if err := json.Unmarshal(raw, &t); err != nil {
			http.Error(w, "invalid telemetry: "+err.Error(), http.StatusBadRequest)
			return
		}
		if t.ReceivedAt.IsZero() {
			t.ReceivedAt = received
		}
		if t.ProcessedAt.IsZero() {
			t.ProcessedAt = time.Now().UTC()
		}
		if err := t.Validate(); err != nil {
			http.Error(w, "invalid telemetry: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}
		evs, err := s.deps.Pipeline.Process(ctx, t)
		if err != nil {
			internalError(w, err)
			return
		}
		accepted++
		for _, e := range evs {
			emitted++
			s.Inject(e)
		}
		if s.deps.PG != nil {
			_ = s.deps.PG.AppendTelemetry(ctx, t)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"accepted": accepted, "events": emitted})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tenant, vehicle, typ := q.Get("tenant"), q.Get("vehicle"), q.Get("type")
	if tenant == "" {
		http.Error(w, "tenant required", http.StatusBadRequest)
		return
	}
	var out []events.Event
	ctx := r.Context()
	if s.deps.PG != nil {
		var err error
		out, err = s.deps.PG.RecentEvents(ctx, tenant, vehicle, typ, 100)
		if err != nil {
			internalError(w, err)
			return
		}
	} else if s.deps.Bus != nil {
		for _, e := range s.deps.Bus.Snapshot() {
			if e.TenantID != tenant {
				continue
			}
			if vehicle != "" && e.VehicleID != vehicle {
				continue
			}
			if typ != "" && e.Type != typ {
				continue
			}
			out = append(out, e)
		}
	}
	if out == nil {
		out = []events.Event{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleVehicle(w http.ResponseWriter, r *http.Request) {
	// GET /v1/vehicles/{id}/state
	rest := strings.TrimPrefix(r.URL.Path, "/v1/vehicles/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[1] != "state" || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	st, ok, err := s.deps.States.Get(r.Context(), parts[0])
	if err != nil {
		internalError(w, err)
		return
	}
	if !ok {
		http.Error(w, "unknown vehicle", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func (s *Server) handleGeofences(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list := s.deps.Geofences.List()
		if list == nil {
			list = []geo.Geofence{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	case http.MethodPost:
		var f geo.Geofence
		if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := s.deps.Geofences.Add(r.Context(), f); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		s.deps.Geo.SetFences(s.deps.Geofences.List())
		w.WriteHeader(http.StatusCreated)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTriggers(w http.ResponseWriter, r *http.Request) {
	tr := s.deps.Rules.Triggers()
	if tr == nil {
		tr = []rules.Trigger{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tr)
}

// Inject republishes an externally-received event (NATS forward) to WS + PG.
func (s *Server) Inject(e events.Event) {
	s.hub.Broadcast(e)
	if s.deps.PG != nil {
		_ = s.deps.PG.AppendEvent(context.Background(), e)
	}
}

func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	list := s.deps.Registry.List()
	// HTTP ingestion is always local to this process — report honestly.
	list = append(list, map[string]any{"name": "http-ingestion", "state": "HEALTHY", "lastSeen": time.Now().UTC().Format(time.RFC3339)})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.hub.Add(c)
	defer func() {
		s.hub.Remove(c)
		_ = c.Close()
	}()
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			return
		}
	}
}
