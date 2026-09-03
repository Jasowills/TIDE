// Package state answers "what do we currently believe is happening" (§2.5).
// State is derived from telemetry, never equal to it. Transitions use
// .started/.ended semantics; identical consecutive states emit nothing
// (T031 property test). Hot state is disposable: Redis dies → rebuild from
// durable telemetry/events, never "we lost state" (§2.6).
package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

type Motion string

const (
	MotionUnknown Motion = "unknown"
	MotionMoving  Motion = "moving"
	MotionIdle    Motion = "idle"
	MotionStopped Motion = "stopped"
)

type Presence string

const (
	PresenceOnline  Presence = "online"
	PresenceOffline Presence = "offline"
)

// MovingThresholdKmh: speed above this is moving. Hysteresis band [3,5]
// avoids flapping at the boundary.
const (
	MovingThresholdKmh = 5.0
	StoppedThresholdKmh = 3.0
	DefaultCadenceSecs  = 300
	OfflineMultiplier   = 3
)

// VehicleState is the current belief about one vehicle.
type VehicleState struct {
	TenantID     string    `json:"tenantId"`
	VehicleID    string    `json:"vehicleId"`
	Motion       Motion    `json:"motion"`
	Presence     Presence  `json:"presence"`
	Ignition     *bool     `json:"ignition,omitempty"`
	SpeedKmh     *float64  `json:"speedKmh,omitempty"`
	Lat          float64   `json:"lat"`
	Lng          float64   `json:"lng"`
	LastSeen     time.Time `json:"lastSeen"`
	TripID       string    `json:"tripId,omitempty"`
	Geofences    []string  `json:"geofences,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Transition is a state change worth emitting downstream.
type Transition struct {
	Dimension string // "motion" | "presence" | "ignition" | "trip"
	From      string
	To        string
	At        time.Time
}

// Engine derives state from telemetry. Pure logic (no I/O) lives in Apply;
// Store handles persistence.
type Engine struct{}

func classifyMotion(prev Motion, t ctelemetry.Telemetry) Motion {
	if t.SpeedKmh == nil {
		if prev == "" {
			return MotionUnknown
		}
		return prev
	}
	s := *t.SpeedKmh
	// Hysteresis: leaving moving requires dropping below 3; entering above 5.
	if prev == MotionMoving && s >= StoppedThresholdKmh {
		return MotionMoving
	}
	if s > MovingThresholdKmh {
		return MotionMoving
	}
	if s < StoppedThresholdKmh {
		if t.Ignition != nil && *t.Ignition {
			return MotionIdle
		}
		return MotionStopped
	}
	// In the 3–5 band: ignition decides, else keep previous.
	if t.Ignition != nil {
		if *t.Ignition {
			return MotionIdle
		}
		return MotionStopped
	}
	if prev == "" {
		return MotionUnknown
	}
	return prev
}

// Apply folds one telemetry point into state, returning transitions.
// Late/out-of-order data never corrupts hot state: points older than the
// current LastSeen update nothing time-sensitive (watermark = LastSeen).
func (Engine) Apply(prev VehicleState, t ctelemetry.Telemetry) (VehicleState, []Transition) {
	next := prev
	if next.Motion == "" {
		next.Motion = MotionUnknown
	}
	if next.Presence == "" {
		next.Presence = PresenceOnline
	}
	next.TenantID, next.VehicleID = t.TenantID, t.VehicleID
	var out []Transition

	late := !prev.LastSeen.IsZero() && t.Timestamp.Before(prev.LastSeen)
	if !late {
		if m := classifyMotion(prev.Motion, t); m != prev.Motion {
			out = append(out, Transition{"motion", string(prev.Motion), string(m), t.Timestamp})
			next.Motion = m
		}
		next.Lat, next.Lng = t.Location.Lat, t.Location.Lng
		next.SpeedKmh = t.SpeedKmh
		if t.Ignition != nil {
			prevStr := ""
			if prev.Ignition != nil && *prev.Ignition {
				prevStr = "on"
			} else if prev.Ignition != nil {
				prevStr = "off"
			}
			curStr := "off"
			if *t.Ignition {
				curStr = "on"
			}
			if prevStr != curStr {
				out = append(out, Transition{"ignition", prevStr, curStr, t.Timestamp})
			}
			next.Ignition = t.Ignition
		}
		next.LastSeen = t.Timestamp
		// Any fresh data means online.
		if prev.Presence == PresenceOffline {
			out = append(out, Transition{"presence", "offline", "online", t.Timestamp})
			next.Presence = PresenceOnline
		} else {
			next.Presence = PresenceOnline
		}
	}
	next.UpdatedAt = time.Now().UTC()
	return next, out
}

// CheckOffline applies per-device expected cadence (§42, T032): a device is
// offline only after OfflineMultiplier × its own cadence of silence.
func CheckOffline(s VehicleState, cadenceSecs int, now time.Time) (VehicleState, *Transition) {
	if cadenceSecs <= 0 {
		cadenceSecs = DefaultCadenceSecs
	}
	if s.LastSeen.IsZero() || s.Presence == PresenceOffline {
		return s, nil
	}
	limit := time.Duration(cadenceSecs*OfflineMultiplier) * time.Second
	if now.Sub(s.LastSeen) > limit {
		s.Presence = PresenceOffline
		return s, &Transition{"presence", "online", "offline", now}
	}
	return s, nil
}

// Store persists hot state. MemoryStore for tests/dev; RedisStore for prod.
// Both are disposable caches — RebuildFrom replays durable telemetry.
type Store interface {
	Get(ctx context.Context, vehicleID string) (VehicleState, bool, error)
	Set(ctx context.Context, s VehicleState) error
}

type MemoryStore struct {
	mu sync.RWMutex
	m  map[string]VehicleState
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{m: map[string]VehicleState{}} }

func (s *MemoryStore) Get(_ context.Context, vehicleID string) (VehicleState, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[vehicleID]
	return v, ok, nil
}

func (s *MemoryStore) Set(_ context.Context, v VehicleState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[v.VehicleID] = v
	return nil
}

// ListIDs returns tracked vehicle ids (offline sweeper).
func (s *MemoryStore) ListIDs(_ context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for id := range s.m {
		out = append(out, id)
	}
	return out, nil
}

type RedisStore struct {
	rdb *redis.Client
}

func NewRedisStore(addr string) *RedisStore {
	return &RedisStore{rdb: redis.NewClient(&redis.Options{Addr: addr})}
}

func (s *RedisStore) Get(ctx context.Context, vehicleID string) (VehicleState, bool, error) {
	raw, err := s.rdb.Get(ctx, "tide:state:"+vehicleID).Bytes()
	if err == redis.Nil {
		return VehicleState{}, false, nil
	}
	if err != nil {
		return VehicleState{}, false, err
	}
	var v VehicleState
	if err := json.Unmarshal(raw, &v); err != nil {
		return VehicleState{}, false, fmt.Errorf("state: corrupt cached state: %w", err)
	}
	return v, true, nil
}

func (s *RedisStore) Set(ctx context.Context, v VehicleState) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, "tide:state:"+v.VehicleID, raw, 0).Err()
}

// ListIDs scans hot-state keys (offline sweeper; SCAN, no KEYS).
func (s *RedisStore) ListIDs(ctx context.Context) ([]string, error) {
	var out []string
	iter := s.rdb.Scan(ctx, 0, "tide:state:*", 100).Iterator()
	for iter.Next(ctx) {
		out = append(out, strings.TrimPrefix(iter.Val(), "tide:state:"))
	}
	return out, iter.Err()
}

// RebuildFrom reconstructs disposable hot state from durable telemetry
func RebuildFrom(ctx context.Context, store Store, log []ctelemetry.Telemetry) error {
	var eng Engine
	byVehicle := map[string][]ctelemetry.Telemetry{}
	for _, t := range log {
		byVehicle[t.VehicleID] = append(byVehicle[t.VehicleID], t)
	}
	for vid, points := range byVehicle {
		// Event-time order: final state must not depend on arrival order.
		for i := 1; i < len(points); i++ {
			for j := i; j > 0 && points[j].Timestamp.Before(points[j-1].Timestamp); j-- {
				points[j], points[j-1] = points[j-1], points[j]
			}
		}
		var cur VehicleState
		for _, p := range points {
			cur, _ = eng.Apply(cur, p)
		}
		cur.VehicleID = vid
		if err := store.Set(ctx, cur); err != nil {
			return err
		}
	}
	return nil
}
