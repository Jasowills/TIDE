// Package rules is the versioned, immutable rule engine (§2.7, T060–T062).
// Rules are YAML (when/for/then) with a persisted evaluation trace per
// trigger. Safety valves (cooldown, dedupe, max-actions/hour) ship with the
// engine, not after. Publishing v2 never mutates v1's recorded behavior.
package rules

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/tide-telematics/tide/internal/state"
	"github.com/tide-telematics/tide/internal/webhooks"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
	"github.com/tide-telematics/tide/schemas/events"
	"gopkg.in/yaml.v3"
)

// Condition matches a numeric field on telemetry (speedKmh, fuelLevel, …)
// or state (stateSpeedKmh). Ops: >, >=, <, <=, ==.
type Condition struct {
	Field string  `yaml:"field"`
	Op    string  `yaml:"op"`
	Value float64 `yaml:"value"`
}

type When struct {
	EventType  string      `yaml:"eventType"`
	Conditions []Condition `yaml:"conditions,omitempty"`
}

type Then struct {
	Emit    string `yaml:"emit,omitempty"` // event type, e.g. incident.created
	Webhook string `yaml:"webhook,omitempty"`
	Secret  string `yaml:"secret,omitempty"`
}

// Spec is the rule DSL (T061: when/for/then).
type Spec struct {
	ID               string `yaml:"id"`
	Version          string `yaml:"version"`
	When             When   `yaml:"when"`
	Then             Then   `yaml:"then"`
	CooldownSecs     int    `yaml:"cooldownSecs"`
	MaxActionsPerHour int   `yaml:"maxActionsPerHour"`
}

// Rule is one published, immutable version.
type Rule struct {
	Spec      Spec
	Published time.Time
}

// Trigger is the persisted evaluation trace (§2.7: rule id, version, matched
// inputs, conditions, actions taken).
type Trigger struct {
	RuleID         string
	RuleVersion    string
	VehicleID      string
	At             time.Time
	MatchedInputs  map[string]any
	ConditionsDesc []string
	ActionsTaken   []string
}

func ParseSpec(data []byte) (Spec, error) {
	var s Spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return Spec{}, fmt.Errorf("rules: parse: %w", err)
	}
	if s.ID == "" || s.Version == "" || s.When.EventType == "" {
		return Spec{}, fmt.Errorf("rules: id, version, when.eventType required")
	}
	for _, c := range s.When.Conditions {
		switch c.Op {
		case ">", ">=", "<", "<=", "==":
		default:
			return Spec{}, fmt.Errorf("rules: unknown op %q", c.Op)
		}
	}
	if s.MaxActionsPerHour == 0 {
		s.MaxActionsPerHour = 100 // sane default valve
	}
	return s, nil
}

// Engine evaluates published rules. Versions are stored per (id, version);
// Evaluate checks every published version pinned at publish time (T060).
type Engine struct {
	Dispatcher *webhooks.Dispatcher

	mu       sync.Mutex
	rules    map[string]Rule // key id/version
	triggers []Trigger
	lastFire map[string]time.Time // key rule/vehicle
	fireLog  map[string][]time.Time
	now      func() time.Time
}

func NewEngine(d *webhooks.Dispatcher) *Engine {
	return &Engine{
		Dispatcher: d,
		rules:      map[string]Rule{},
		lastFire:   map[string]time.Time{},
		fireLog:    map[string][]time.Time{},
		now:        time.Now,
	}
}

// Publish adds an immutable version. Republishing the same version with
// different content is rejected (T060).
func (e *Engine) Publish(s Spec, at time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	k := s.ID + "/" + s.Version
	if existing, ok := e.rules[k]; ok && !reflect.DeepEqual(existing.Spec, s) {
		return fmt.Errorf("rules: %s is published and immutable", k)
	}
	e.rules[k] = Rule{Spec: s, Published: at}
	return nil
}

func fieldValue(t ctelemetry.Telemetry, s state.VehicleState, field string) (float64, bool) {
	switch field {
	case "speedKmh":
		if t.SpeedKmh == nil {
			return 0, false
		}
		return *t.SpeedKmh, true
	case "stateSpeedKmh":
		if s.SpeedKmh == nil {
			return 0, false
		}
		return *s.SpeedKmh, true
	case "fuelLevel":
		if t.FuelLevelL == nil {
			return 0, false
		}
		return *t.FuelLevelL, true
	}
	return 0, false
}

func matchOp(op string, got, want float64) bool {
	switch op {
	case ">":
		return got > want
	case ">=":
		return got >= want
	case "<":
		return got < want
	case "<=":
		return got <= want
	case "==":
		return got == want
	}
	return false
}

// Evaluate implements pipeline.Rules: match → valves → trace → actions.
func (e *Engine) Evaluate(_ context.Context, t ctelemetry.Telemetry, s state.VehicleState, evs []events.Event) []events.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []events.Event
	now := e.now()
	for _, r := range e.rules {
		spec := r.Spec
		for _, ev := range evs {
			if ev.Type != spec.When.EventType || ev.VehicleID != t.VehicleID {
				continue
			}
			matched := map[string]any{"event": ev.Type, "eventId": ev.ID}
			var descs []string
			ok := true
			for _, c := range spec.When.Conditions {
				v, found := fieldValue(t, s, c.Field)
				descs = append(descs, fmt.Sprintf("%s %s %v (saw %v)", c.Field, c.Op, c.Value, v))
				matched[c.Field] = v
				if !found || !matchOp(c.Op, v, c.Value) {
					ok = false
				}
			}
			if !ok {
				continue
			}
			// Safety valves (T062): cooldown + max-actions/hour per rule+vehicle.
			vk := spec.ID + "/" + spec.Version + "/" + t.VehicleID
			if spec.CooldownSecs > 0 {
				if last, seen := e.lastFire[vk]; seen && now.Sub(last).Seconds() < float64(spec.CooldownSecs) {
					continue
				}
			}
			hourAgo := now.Add(-time.Hour)
			var recent []time.Time
			for _, ts := range e.fireLog[vk] {
				if ts.After(hourAgo) {
					recent = append(recent, ts)
				}
			}
			e.fireLog[vk] = recent
			if len(recent) >= spec.MaxActionsPerHour {
				continue // valve held: malfunctioning device can't spam downstream
			}
			var actions []string
			if spec.Then.Emit != "" {
				emit := events.Event{
					ID: events.DeterministicID(spec.Then.Emit, t.VehicleID, t.Timestamp,
						spec.ID+"/"+spec.Version),
					Type: spec.Then.Emit, TenantID: t.TenantID, VehicleID: t.VehicleID,
					Timestamp: t.Timestamp, RuleID: spec.ID, RuleVersion: spec.Version,
					CorrelationID: t.Metadata.CorrelationID, CausationID: ev.ID,
					Payload: map[string]any{
						"ruleId": spec.ID, "ruleVersion": spec.Version,
						"matchedEvent": ev.Type, "matchedInputs": matched,
					},
					SchemaVersion: events.CurrentSchemaVersion,
				}
				if err := emit.Validate(); err == nil {
					out = append(out, emit)
					actions = append(actions, "emit:"+spec.Then.Emit)
				}
			}
			if spec.Then.Webhook != "" && e.Dispatcher != nil {
				// Dispatch the emitted (or triggering) event.
				target := ev
				for _, o := range out {
					if o.RuleID == spec.ID {
						target = o
					}
				}
				if err := e.Dispatcher.Dispatch(spec.Then.Webhook, spec.Then.Secret, target); err != nil {
					actions = append(actions, "webhook:failed")
				} else {
					actions = append(actions, "webhook:delivered")
				}
			}
			e.lastFire[vk] = now
			e.fireLog[vk] = append(e.fireLog[vk], now)
			e.triggers = append(e.triggers, Trigger{
				RuleID: spec.ID, RuleVersion: spec.Version, VehicleID: t.VehicleID,
				At: now, MatchedInputs: matched, ConditionsDesc: descs, ActionsTaken: actions,
			})
		}
	}
	return out
}

// Triggers returns the evaluation trace (audit).
func (e *Engine) Triggers() []Trigger {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]Trigger{}, e.triggers...)
}
