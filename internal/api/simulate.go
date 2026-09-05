package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tide-telematics/tide/simulator"
	"github.com/tide-telematics/tide/simulator/faults"
	"github.com/tide-telematics/tide/simulator/generators"
	"github.com/tide-telematics/tide/internal/pipeline"
)

// simulateBounds keep HTTP runs interactive. Larger runs belong to the CLI,
// which streams instead of holding a request open.
const (
	maxSimVehicles = 50
	maxSimPoints   = 20000
)

type simRequest struct {
	Scenario string        `json:"scenario"`
	Vehicles int           `json:"vehicles"`
	Seed     int64         `json:"seed"`
	Tenant   string        `json:"tenant"`
	Faults   faults.Config `json:"faults"`
}

type simStatus struct {
	ID       string `json:"id"`
	State    string `json:"state"` // running | done | cancelled | failed
	Scenario string `json:"scenario,omitempty"`
	Vehicles int    `json:"vehicles,omitempty"`
	Points   int    `json:"points,omitempty"`
	Accepted int    `json:"accepted,omitempty"`
	Events   int    `json:"events,omitempty"`
	Error    string `json:"error,omitempty"`
}

type simRegistry struct {
	mu      sync.Mutex
	runs    map[string]*simStatus
	cancels map[string]context.CancelFunc
	seq     int64
}

func newSimRegistry() *simRegistry {
	return &simRegistry{runs: map[string]*simStatus{}, cancels: map[string]context.CancelFunc{}}
}

func (r *simRegistry) set(id string, fn func(*simStatus)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st, ok := r.runs[id]; ok {
		fn(st)
	}
}

func (r *simRegistry) get(id string) (simStatus, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.runs[id]
	if !ok {
		return simStatus{}, false
	}
	return *st, true
}

// POST /v1/simulate starts a bounded FleetSim run through the production
// pipeline (same generators the CLI uses). Returns 202 + run id; poll
// GET /v1/simulate/:id; DELETE /v1/simulate/:id cancels.
func (s *Server) handleSimulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req simRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Scenario == "" {
		req.Scenario = "mixed"
	}
	if req.Tenant == "" {
		req.Tenant = "default"
	}
	if req.Vehicles <= 0 {
		req.Vehicles = 10
	}
	if req.Vehicles > maxSimVehicles {
		http.Error(w, fmt.Sprintf("vehicles capped at %d for HTTP runs (use the CLI beyond that)", maxSimVehicles), http.StatusUnprocessableEntity)
		return
	}
	for _, rate := range []float64{req.Faults.DuplicateRate, req.Faults.LateRate, req.Faults.MissingRate, req.Faults.OutOfOrderRate, req.Faults.OfflineVehicles} {
		if rate < 0 || rate > 1 {
			http.Error(w, "fault rates must be 0-1", http.StatusUnprocessableEntity)
			return
		}
	}
	raw, err := simulator.Get(req.Scenario)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	scen, err := generators.LoadScenario(raw)
	if err != nil {
		internalError(w, err)
		return
	}
	s.sims.mu.Lock()
	s.sims.seq++
	id := fmt.Sprintf("sim-%d", s.sims.seq)
	st := &simStatus{ID: id, State: "running", Scenario: req.Scenario, Vehicles: req.Vehicles}
	ctx, cancel := context.WithCancel(context.Background())
	s.sims.runs[id] = st
	s.sims.cancels[id] = cancel
	s.sims.mu.Unlock()

	go s.runSim(ctx, st, scen, req)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(st)
}

func (s *Server) runSim(ctx context.Context, st *simStatus, scen generators.Scenario, req simRequest) {
	pts := generators.Generate(scen, req.Vehicles, req.Seed, req.Tenant, time.Now().UTC())
	pts = faults.Apply(pts, req.Faults, req.Seed)
	n := len(pts)
	if n > maxSimPoints {
		n = maxSimPoints
		pts = pts[:n]
	}
	s.sims.set(st.ID, func(s *simStatus) { s.Points = n })
	for _, pt := range pts {
		select {
		case <-ctx.Done():
			s.sims.set(st.ID, func(s *simStatus) { s.State = "cancelled" })
			return
		default:
		}
		pt.ProcessedAt = time.Now().UTC()
		evs, _, err := s.deps.Pipeline.Process(ctx, pt)
		if err != nil {
			var perr *pipeline.PublishError
			if errors.As(err, &perr) {
				// Bus down mid-run: persist computed events, keep going.
				for _, e := range perr.Events {
					st.Events++
					s.Inject(e)
				}
				st.Accepted++
				continue
			}
			s.sims.set(st.ID, func(s *simStatus) { s.State, s.Error = "failed", "processing failed" })
			return
		}
		s.sims.set(st.ID, func(s *simStatus) { s.Accepted++ })
		for _, e := range evs {
			s.sims.set(st.ID, func(s *simStatus) { s.Events++ })
			s.Inject(e)
		}
	}
	s.sims.set(st.ID, func(s *simStatus) { s.State = "done" })
}

func (s *Server) handleSimStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/simulate/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	st, ok := s.sims.get(id)
	if !ok {
		http.Error(w, "unknown run", http.StatusNotFound)
		return
	}
	if r.Method == http.MethodDelete {
		s.sims.mu.Lock()
		if cancel, has := s.sims.cancels[id]; has {
			cancel()
		}
		s.sims.mu.Unlock()
		st, _ = s.sims.get(id)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}
