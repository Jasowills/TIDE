// Package detectors turns state+telemetry into transition events.
// T041 speeding (.started/.continued/.ended) is the first real event;
// T042 idling + basic trips (ignition+movement heuristic, configurable).
// All detectors are pure functions of (tracker, telemetry, now) so replay
// reuses them byte-for-byte (Architecture §2.8).
package detectors

import (
	"math"
	"time"

	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
	"github.com/tide-telematics/tide/schemas/events"
	"github.com/tide-telematics/tide/internal/state"
)

type Config struct {
	SpeedLimitKmh    float64
	SpeedForSecs     int
	SpeedContinuedEvery time.Duration
	IdleForSecs      int
	TripEndSecs      int
}

func DefaultConfig() Config {
	return Config{
		SpeedLimitKmh: 100, SpeedForSecs: 10,
		SpeedContinuedEvery: time.Minute,
		IdleForSecs:   300, TripEndSecs: 300,
	}
}

type speedingTrack struct {
	since       time.Time
	started     bool
	lastContinued time.Time
}

type idleTrack struct {
	since   time.Time
	started bool
}

type tripTrack struct {
	active    bool
	id        string
	startedAt time.Time
	lastMove  time.Time
	distanceKm float64
	lastLat, lastLng float64
	hasLast   bool
}

// Tracker holds per-vehicle detector memory. One per pipeline instance.
type Tracker struct {
	cfg      Config
	speeding map[string]*speedingTrack
	idle     map[string]*idleTrack
	trips    map[string]*tripTrack
}

func NewTracker(cfg Config) *Tracker {
	return &Tracker{cfg: cfg,
		speeding: map[string]*speedingTrack{},
		idle:     map[string]*idleTrack{},
		trips:    map[string]*tripTrack{}}
}

func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * R * math.Asin(math.Sqrt(a))
}

// Detect runs all detectors for one telemetry point + resulting state.
func (tr *Tracker) Detect(t ctelemetry.Telemetry, s state.VehicleState) []events.Event {
	var out []events.Event
	out = append(out, tr.speedingDetect(t, s)...)
	out = append(out, tr.idleDetect(t, s)...)
	out = append(out, tr.tripDetect(t, s)...)
	return out
}

func mkEvent(typ string, t ctelemetry.Telemetry, cause string, payload map[string]any) events.Event {
	return events.Event{
		ID:            events.DeterministicID(typ, t.VehicleID, t.Timestamp, cause),
		Type:          typ,
		TenantID:      t.TenantID,
		VehicleID:     t.VehicleID,
		Timestamp:     t.Timestamp,
		CorrelationID: t.Metadata.CorrelationID,
		CausationID:   t.ID,
		Payload:       payload,
		SchemaVersion: events.CurrentSchemaVersion,
	}
}

func speedOf(t ctelemetry.Telemetry) float64 {
	if t.SpeedKmh == nil {
		return 0
	}
	return *t.SpeedKmh
}

func (tr *Tracker) speedingDetect(t ctelemetry.Telemetry, _ state.VehicleState) []events.Event {
	st := tr.speeding[t.VehicleID]
	if st == nil {
		st = &speedingTrack{}
		tr.speeding[t.VehicleID] = st
	}
	over := speedOf(t) > tr.cfg.SpeedLimitKmh
	var out []events.Event
	now := t.Timestamp
	switch {
	case over && !st.started:
		if st.since.IsZero() {
			st.since = now
		}
		if now.Sub(st.since).Seconds() >= float64(tr.cfg.SpeedForSecs) {
			st.started = true
			st.lastContinued = now
			out = append(out, mkEvent("vehicle.speeding.started", t, "speeding",
				map[string]any{"speedKmh": speedOf(t), "limitKmh": tr.cfg.SpeedLimitKmh}))
		}
	case over && st.started:
		if now.Sub(st.lastContinued) >= tr.cfg.SpeedContinuedEvery {
			st.lastContinued = now
			out = append(out, mkEvent("vehicle.speeding.continued", t, "speeding",
				map[string]any{"speedKmh": speedOf(t), "limitKmh": tr.cfg.SpeedLimitKmh}))
		}
	case !over && st.started:
		st.started = false
		st.since = time.Time{}
		out = append(out, mkEvent("vehicle.speeding.ended", t, "speeding",
			map[string]any{"speedKmh": speedOf(t), "limitKmh": tr.cfg.SpeedLimitKmh}))
	case !over && !st.started:
		st.since = time.Time{}
	}
	return out
}

func (tr *Tracker) idleDetect(t ctelemetry.Telemetry, s state.VehicleState) []events.Event {
	st := tr.idle[t.VehicleID]
	if st == nil {
		st = &idleTrack{}
		tr.idle[t.VehicleID] = st
	}
	var out []events.Event
	if s.Motion == state.MotionIdle {
		if st.since.IsZero() {
			st.since = t.Timestamp
		}
		if !st.started && t.Timestamp.Sub(st.since).Seconds() >= float64(tr.cfg.IdleForSecs) {
			st.started = true
			out = append(out, mkEvent("vehicle.idling.started", t, "idling", nil))
		}
	} else {
		if st.started {
			out = append(out, mkEvent("vehicle.idling.ended", t, "idling", nil))
		}
		st.started = false
		st.since = time.Time{}
	}
	return out
}

func (tr *Tracker) tripDetect(t ctelemetry.Telemetry, s state.VehicleState) []events.Event {
	st := tr.trips[t.VehicleID]
	if st == nil {
		st = &tripTrack{}
		tr.trips[t.VehicleID] = st
	}
	var out []events.Event
	moving := s.Motion == state.MotionMoving
	if !st.active && moving {
		st.active = true
		st.id = events.DeterministicID("trip", t.VehicleID, t.Timestamp, "start")
		st.startedAt = t.Timestamp
		st.lastMove = t.Timestamp
		st.hasLast = false
		out = append(out, mkEvent("vehicle.trip.started", t, "trip",
			map[string]any{"tripId": st.id}))
	}
	if st.active {
		if st.hasLast {
			st.distanceKm += haversineKm(st.lastLat, st.lastLng, t.Location.Lat, t.Location.Lng)
		}
		st.lastLat, st.lastLng, st.hasLast = t.Location.Lat, t.Location.Lng, true
		if moving {
			st.lastMove = t.Timestamp
		}
		ignOff := t.Ignition != nil && !*t.Ignition
		expired := t.Timestamp.Sub(st.lastMove).Seconds() >= float64(tr.cfg.TripEndSecs)
		if ignOff || expired {
			st.active = false
			out = append(out, mkEvent("vehicle.trip.ended", t, "trip",
				map[string]any{"tripId": st.id, "distanceKm": math.Round(st.distanceKm*100) / 100}))
		}
	}
	return out
}
