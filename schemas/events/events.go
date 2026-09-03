// Package events defines the canonical event contract (Architecture §2.5).
// Naming is always transition-based: <noun>.<started|continued|ended>.
// Raw packets are never events. Delivery: at-least-once + idempotent
// consumers + deterministic event identity — never claim exactly-once.
package events

import (
	"fmt"
	"strings"
	"time"
)

const CurrentSchemaVersion = 1

var validSuffixes = map[string]bool{
	"started": true, "continued": true, "ended": true,
	"entered": true, "exited": true, // geofence transitions (QA §3.1.5)
	"created": true, // one-time occurrences with no lifecycle (e.g. incident.created, PRD §1.4)
}

// Event is a state transition derived from telemetry, not telemetry itself.
type Event struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	TenantID      string         `json:"tenantId"`
	VehicleID     string         `json:"vehicleId"`
	Timestamp     time.Time      `json:"timestamp"`
	RuleID        string         `json:"ruleId,omitempty"`
	RuleVersion   string         `json:"ruleVersion,omitempty"`
	CorrelationID string         `json:"correlationId"`
	CausationID   string         `json:"causationId,omitempty"`
	Payload       map[string]any `json:"payload,omitempty"`
	SchemaVersion int            `json:"schemaVersion"`
}

// DeterministicID makes redelivery idempotent: same cause → same id.
func DeterministicID(eventType, vehicleID string, ts time.Time, cause string) string {
	return fmt.Sprintf("%s:%s:%s:%s", eventType, vehicleID, ts.UTC().Format(time.RFC3339Nano), cause)
}

func (e Event) Validate() error {
	if e.ID == "" || e.Type == "" || e.TenantID == "" || e.VehicleID == "" {
		return fmt.Errorf("id, type, tenantId, vehicleId required")
	}
	parts := strings.Split(e.Type, ".")
	if len(parts) < 2 || !validSuffixes[parts[len(parts)-1]] {
		return fmt.Errorf("type %q must end with a transition suffix (.started/.continued/.ended/.entered/.exited/.created)", e.Type)
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("timestamp required")
	}
	if e.CorrelationID == "" {
		return fmt.Errorf("correlationId required")
	}
	if e.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("unsupported schemaVersion %d", e.SchemaVersion)
	}
	return nil
}

// NATS subject per §37 topic list: tide.events.<tenant>.<type>
func (e Event) Subject() string {
	return fmt.Sprintf("tide.events.%s.%s", e.TenantID, e.Type)
}
