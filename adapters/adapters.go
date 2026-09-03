// Package adapters defines the TelemetrySource contract (Architecture §2.4).
// Provider logic lives ONLY inside /adapters/<provider> — enforced by
// scripts/check-provider-isolation.sh from commit #1.
package adapters

import "context"

type HealthState string

const (
	StateConfigured   HealthState = "CONFIGURED"
	StateConnecting   HealthState = "CONNECTING"
	StateHealthy      HealthState = "HEALTHY"
	StateDegraded     HealthState = "DEGRADED"
	StateReconnecting HealthState = "RECONNECTING"
	StateFailed       HealthState = "FAILED"
)

type HealthStatus struct {
	State      HealthState `json:"state"`
	Message    string      `json:"message,omitempty"`
	DeviceCount int        `json:"deviceCount,omitempty"`
	MsgPerSec  float64     `json:"msgPerSec,omitempty"`
}

type Capabilities struct {
	LiveTelemetry  bool `json:"liveTelemetry"`
	HistoricalData bool `json:"historicalData"`
	DeviceList     bool `json:"deviceList"`
	Events         bool `json:"events"`
	Webhooks       bool `json:"webhooks"`
	Commands       bool `json:"commands"`
}

type Vehicle struct {
	ProviderID string `json:"providerId"`
	Name       string `json:"name"`
}

// TelemetryHandler receives raw provider payloads; the adapter normalizes to
// canonical telemetry before invoking it (FleetSim-indistinguishable at the
// ingestion boundary, §2.9). Topic may be "" for non-topic transports.
type TelemetryHandler func(ctx context.Context, topic string, payload []byte) error

type TelemetrySource interface {
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	Subscribe(ctx context.Context, handler TelemetryHandler) error
	ListVehicles(ctx context.Context) ([]Vehicle, error)
	GetVehicle(ctx context.Context, id string) (*Vehicle, error)
	Health(ctx context.Context) HealthStatus
	Capabilities() Capabilities
}
