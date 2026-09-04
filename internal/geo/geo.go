// Package geo owns geofences (T050/T051): PostGIS model + point-in-polygon
// evaluation + enter/exit state tracking with exactly-one semantics.
// Hot path uses pure-Go ray casting over candidate geofences; PostGIS
// ST_Contains is the durable query path (T052: pre-filter before full
// evaluation only where a benchmark shows need — the seam is CandidateFilter).
package geo

import (
	"context"
	"sync"

	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
	"github.com/tide-telematics/tide/schemas/events"
)

type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type Geofence struct {
	ID       string  `json:"id"`
	TenantID string  `json:"tenantId"`
	Name     string  `json:"name"`
	Polygon  []Point `json:"polygon"`
}

// Contains implements ray-casting point-in-polygon.
func (g Geofence) Contains(p Point) bool {
	inside := false
	n := len(g.Polygon)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		xi, yi := g.Polygon[i].Lng, g.Polygon[i].Lat
		xj, yj := g.Polygon[j].Lng, g.Polygon[j].Lat
		if ((yi > p.Lat) != (yj > p.Lat)) &&
			(p.Lng < (xj-xi)*(p.Lat-yi)/(yj-yi)+xi) {
			inside = !inside
		}
	}
	return inside
}

// CandidateFilter narrows geofences before full evaluation (T052 seam).
// Default is a bounding-box pre-filter; replace with spatial index when a
// benchmark justifies it.
func CandidateFilter(fences []Geofence, p Point) []Geofence {
	var out []Geofence
	for _, g := range fences {
		minLat, maxLat := 90.0, -90.0
		minLng, maxLng := 180.0, -180.0
		for _, v := range g.Polygon {
			if v.Lat < minLat {
				minLat = v.Lat
			}
			if v.Lat > maxLat {
				maxLat = v.Lat
			}
			if v.Lng < minLng {
				minLng = v.Lng
			}
			if v.Lng > maxLng {
				maxLng = v.Lng
			}
		}
		if p.Lat >= minLat && p.Lat <= maxLat && p.Lng >= minLng && p.Lng <= maxLng {
			out = append(out, g)
		}
	}
	return out
}

// Tracker holds per-vehicle geofence membership. Enter-then-exit yields
// exactly one entered and one exited, never repeated (QA §3.1.5).
type Tracker struct {
	mu      sync.Mutex
	fences  []Geofence
	members map[string]map[string]bool // vehicleID → fenceID → inside
}

func NewTracker(fences []Geofence) *Tracker {
	return &Tracker{fences: fences, members: map[string]map[string]bool{}}
}

func (t *Tracker) SetFences(fences []Geofence) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.fences = fences
}

func (t *Tracker) Evaluate(ctx context.Context, tel ctelemetry.Telemetry) []events.Event {
	_ = ctx
	t.mu.Lock()
	defer t.mu.Unlock()
	p := Point{Lat: tel.Location.Lat, Lng: tel.Location.Lng}
	mem := t.members[tel.VehicleID]
	if mem == nil {
		mem = map[string]bool{}
		t.members[tel.VehicleID] = mem
	}
	var out []events.Event
	for _, g := range CandidateFilter(t.fences, p) {
		inside := g.Contains(p)
		was := mem[g.ID]
		if inside && !was {
			mem[g.ID] = true
			out = append(out, events.Event{
				ID: events.DeterministicID("vehicle.geofence.entered", tel.VehicleID, tel.Timestamp, g.ID),
				Type: "vehicle.geofence.entered", TenantID: tel.TenantID, VehicleID: tel.VehicleID,
				Timestamp: tel.Timestamp, CorrelationID: tel.Metadata.CorrelationID, CausationID: tel.ID,
				Payload: map[string]any{"geofenceId": g.ID, "geofenceName": g.Name,
				"lat": tel.Location.Lat, "lng": tel.Location.Lng},
				SchemaVersion: events.CurrentSchemaVersion,
			})
		} else if !inside && was {
			// Note: exit detected only when a candidate fence no longer
			// contains the point. Fences outside the bbox can't be exited
			// into — but an exit always passes through the bbox edge, and
			// membership is checked on every point, so check all members:
			// handled below.
			mem[g.ID] = false
			out = append(out, exitEvent(tel, g))
		}
	}
	// Exits from fences that aren't bbox candidates (vehicle left the area).
	for _, g := range t.fences {
		if !mem[g.ID] {
			continue
		}
		stillCandidate := false
		for _, c := range CandidateFilter([]Geofence{g}, p) {
			if c.ID == g.ID {
				stillCandidate = true
			}
		}
		if !stillCandidate {
			mem[g.ID] = false
			out = append(out, exitEvent(tel, g))
		}
	}
	return out
}

func exitEvent(tel ctelemetry.Telemetry, g Geofence) events.Event {
	return events.Event{
		ID: events.DeterministicID("vehicle.geofence.exited", tel.VehicleID, tel.Timestamp, g.ID),
		Type: "vehicle.geofence.exited", TenantID: tel.TenantID, VehicleID: tel.VehicleID,
		Timestamp: tel.Timestamp, CorrelationID: tel.Metadata.CorrelationID, CausationID: tel.ID,
		Payload: map[string]any{"geofenceId": g.ID, "geofenceName": g.Name,
			"lat": tel.Location.Lat, "lng": tel.Location.Lng},
		SchemaVersion: events.CurrentSchemaVersion,
	}
}
