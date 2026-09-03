// Package telemetry defines the canonical telemetry contract (Architecture §2.3).
// Single most important artifact: everything downstream depends on it.
// Units fixed: km/h, meters, °C, litres, WGS84, UTC. Conversion is an
// adapter responsibility, never core. Raw payload is never discarded.
// Schema changes require ADR + second reviewer (ADR-001 §7); additive only.
package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const CurrentSchemaVersion = 1

type Location struct {
	Lat      float64  `json:"lat"`
	Lng      float64  `json:"lng"`
	Altitude *float64 `json:"altitude,omitempty"`
	Accuracy *float64 `json:"accuracy,omitempty"`
}

type Source struct {
	Provider string `json:"provider"`
	Protocol string `json:"protocol"`
	DeviceID string `json:"deviceId"`
}

type Metadata struct {
	CorrelationID string  `json:"correlationId"`
	SchemaVersion int     `json:"schemaVersion"`
	Sequence      *int64  `json:"sequence,omitempty"`
	Quality       string  `json:"quality"` // "good" | "degraded:<reason>"
}

// Telemetry is one canonical vehicle observation. The three timestamps are
// mandatory and distinct: event time, received_at, processed_at.
type Telemetry struct {
	ID            string         `json:"id"`
	TenantID      string         `json:"tenantId"`
	VehicleID     string         `json:"vehicleId"`
	DeviceID      string         `json:"deviceId"`
	Timestamp     time.Time      `json:"timestamp"`
	ReceivedAt    time.Time      `json:"receivedAt"`
	ProcessedAt   time.Time      `json:"processedAt"`
	Location      Location       `json:"location"`
	SpeedKmh      *float64       `json:"speed,omitempty"`
	Heading       *float64       `json:"heading,omitempty"`
	Ignition      *bool          `json:"ignition,omitempty"`
	OdometerM     *float64       `json:"odometer,omitempty"`
	FuelLevelL    *float64       `json:"fuelLevel,omitempty"`
	BatteryVolt   *float64       `json:"batteryVoltage,omitempty"`
	Engine        map[string]any `json:"engine,omitempty"`
	Sensors       map[string]any `json:"sensors,omitempty"`
	Raw           map[string]any `json:"raw"`
	Source        Source         `json:"source"`
	Metadata      Metadata       `json:"metadata"`
}

// Validate rejects malformed payloads with a quality reason — never silently
// drops, never panics (T012 acceptance).
func (t Telemetry) Validate() error {
	if t.TenantID == "" {
		return fmt.Errorf("tenantId required")
	}
	if t.VehicleID == "" {
		return fmt.Errorf("vehicleId required")
	}
	if t.DeviceID == "" {
		return fmt.Errorf("deviceId required")
	}
	if t.Timestamp.IsZero() || t.ReceivedAt.IsZero() {
		return fmt.Errorf("timestamp and receivedAt required")
	}
	if t.Location.Lat < -90 || t.Location.Lat > 90 {
		return fmt.Errorf("location.lat out of range")
	}
	if t.Location.Lng < -180 || t.Location.Lng > 180 {
		return fmt.Errorf("location.lng out of range")
	}
	if t.SpeedKmh != nil && *t.SpeedKmh < 0 {
		return fmt.Errorf("speed must be >= 0 km/h")
	}
	if t.Heading != nil && (*t.Heading < 0 || *t.Heading >= 360) {
		return fmt.Errorf("heading must be [0,360)")
	}
	if t.FuelLevelL != nil && *t.FuelLevelL < 0 {
		return fmt.Errorf("fuelLevel must be >= 0 litres")
	}
	if t.Metadata.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("unsupported schemaVersion %d (want %d)", t.Metadata.SchemaVersion, CurrentSchemaVersion)
	}
	if t.Raw == nil {
		return fmt.Errorf("raw payload must be preserved (may be {})")
	}
	return nil
}

// DedupKey implements Architecture §2.5 identity preference:
// (provider, device, sequence) → fallback (provider, device, timestamp, payload hash).
func (t Telemetry) DedupKey() string {
	if t.Metadata.Sequence != nil {
		return fmt.Sprintf("%s|%s|seq:%d", t.Source.Provider, t.Source.DeviceID, *t.Metadata.Sequence)
	}
	raw, _ := json.Marshal(t.Raw)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%s|%s|%s|%s", t.Source.Provider, t.Source.DeviceID,
		t.Timestamp.UTC().Format(time.RFC3339Nano), hex.EncodeToString(sum[:8]))
}
