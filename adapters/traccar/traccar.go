// Package traccar ingests Traccar (T090, REST poll path).
// Version pin (Grilling §5.1): Traccar REST API v6.x positions/devices schema.
// Contract-tested against recorded fixtures in CI — never live Traccar.
// Units: Traccar reports speed in KNOTS; conversion happens here (§2.3).
package traccar

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/tide-telematics/tide/adapters"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

const PinnedAPIVersion = "traccar-6.x"

// Position mirrors the Traccar /api/positions element (subset we consume).
type Position struct {
	ID         int64          `json:"id"`
	DeviceID   int64          `json:"deviceId"`
	Protocol   string         `json:"protocol"`
	DeviceTime time.Time      `json:"deviceTime"`
	FixTime    time.Time      `json:"fixTime"`
	Latitude   float64        `json:"latitude"`
	Longitude  float64        `json:"longitude"`
	Speed      float64        `json:"speed"` // knots
	Attributes map[string]any `json:"attributes"`
}

type Device struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// PositionToTelemetry normalizes one Traccar position to canonical telemetry.
func PositionToTelemetry(tenant string, p Position, now time.Time) (ctelemetry.Telemetry, error) {
	speedKmh := p.Speed * 1.852 // knots → km/h at the adapter boundary
	t := ctelemetry.Telemetry{
		ID: fmt.Sprintf("traccar-%d", p.ID), TenantID: tenant,
		VehicleID: fmt.Sprintf("traccar-%d", p.DeviceID),
		DeviceID:  fmt.Sprintf("traccar-%d", p.DeviceID),
		Timestamp: p.FixTime.UTC(), ReceivedAt: now.UTC(),
		Location:  ctelemetry.Location{Lat: p.Latitude, Lng: p.Longitude},
		SpeedKmh:  &speedKmh,
		Raw:       map[string]any{"protocol": p.Protocol, "attributes": p.Attributes},
		Source:    ctelemetry.Source{Provider: "traccar", Protocol: "traccar-rest", DeviceID: fmt.Sprint(p.DeviceID)},
		Metadata: ctelemetry.Metadata{CorrelationID: fmt.Sprintf("traccar-%d", p.ID),
			SchemaVersion: ctelemetry.CurrentSchemaVersion, Quality: "good"},
	}
	if p.Attributes != nil {
		if v, ok := p.Attributes["ignition"]; ok {
			if b, ok := v.(bool); ok {
				t.Ignition = &b
			}
		}
		if v, ok := p.Attributes["odometer"]; ok {
			if f, ok := toFloat(v); ok {
				t.OdometerM = &f
			}
		}
	}
	if err := t.Validate(); err != nil {
		return ctelemetry.Telemetry{}, fmt.Errorf("traccar: invalid normalized position: %w", err)
	}
	return t, nil
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

// Adapter polls Traccar REST. V1 trust model: in-process, no plugin sandbox.
type Adapter struct {
	baseURL string
	auth    string // basic auth header value
	client  *http.Client
	tenant  string

	mu    sync.RWMutex
	state adapters.HealthState
}

func New(baseURL, user, password, tenant string) *Adapter {
	return &Adapter{
		baseURL: baseURL,
		auth:    "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password)),
		client:  &http.Client{Timeout: 10 * time.Second},
		tenant:  tenant,
		state:   adapters.StateConfigured,
	}
}

func (a *Adapter) setState(s adapters.HealthState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = s
}

func (a *Adapter) Connect(ctx context.Context) error {
	a.setState(adapters.StateConnecting)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/api/server", nil)
	req.Header.Set("Authorization", a.auth)
	resp, err := a.client.Do(req)
	if err != nil {
		a.setState(adapters.StateFailed)
		return fmt.Errorf("traccar: connect: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		a.setState(adapters.StateFailed)
		return fmt.Errorf("traccar: server returned %d", resp.StatusCode)
	}
	a.setState(adapters.StateHealthy)
	return nil
}

func (a *Adapter) Disconnect(_ context.Context) error {
	a.setState(adapters.StateConfigured)
	return nil
}

func (a *Adapter) Subscribe(ctx context.Context, handler adapters.TelemetryHandler) error {
	// REST poll loop (WebSocket live path is a later ticket; poll keeps V1 tissue-thin).
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			positions, err := a.fetchPositions(ctx)
			if err != nil {
				a.setState(adapters.StateDegraded)
				continue
			}
			a.setState(adapters.StateHealthy)
			for _, p := range positions {
				raw, _ := json.Marshal(p)
				if err := handler(ctx, "", raw); err != nil {
					return err
				}
			}
		}
	}
}

func (a *Adapter) fetchPositions(ctx context.Context) ([]Position, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/api/positions", nil)
	req.Header.Set("Authorization", a.auth)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out []Position
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// HandleRaw normalizes one raw position payload (shared with tests).
func (a *Adapter) HandleRaw(raw []byte, now time.Time) (ctelemetry.Telemetry, error) {
	var p Position
	if err := json.Unmarshal(raw, &p); err != nil {
		return ctelemetry.Telemetry{}, err
	}
	return PositionToTelemetry(a.tenant, p, now)
}

func (a *Adapter) ListVehicles(ctx context.Context) ([]adapters.Vehicle, error) {
	if a.Health(ctx).State != adapters.StateHealthy {
		return nil, fmt.Errorf("traccar: not connected")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/api/devices", nil)
	req.Header.Set("Authorization", a.auth)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var devs []Device
	if err := json.NewDecoder(resp.Body).Decode(&devs); err != nil {
		return nil, err
	}
	var out []adapters.Vehicle
	for _, d := range devs {
		out = append(out, adapters.Vehicle{ProviderID: fmt.Sprint(d.ID), Name: d.Name})
	}
	return out, nil
}

func (a *Adapter) GetVehicle(ctx context.Context, id string) (*adapters.Vehicle, error) {
	all, err := a.ListVehicles(ctx)
	if err != nil {
		return nil, err
	}
	for _, v := range all {
		if v.ProviderID == id {
			v := v
			return &v, nil
		}
	}
	return nil, fmt.Errorf("traccar: unknown device %s", id)
}

func (a *Adapter) Health(_ context.Context) adapters.HealthStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return adapters.HealthStatus{State: a.state}
}

func (a *Adapter) Capabilities() adapters.Capabilities {
	return adapters.Capabilities{LiveTelemetry: true, HistoricalData: true, DeviceList: true}
}
