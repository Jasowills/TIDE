// Adapter is the live MQTT subscriber. Mapping (zero-Go-code path) is in
// mapping.go; this file only owns connection lifecycle with exponential
// backoff + jitter (Architecture §2.4).
package mqtt

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/tide-telematics/tide/adapters"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

type Adapter struct {
	cfg     MappingConfig
	emit    func(ctx context.Context, t ctelemetry.Telemetry) error
	mu      sync.RWMutex
	state   adapters.HealthState
	message string
	count   int64
	start   time.Time
	client  paho.Client
}

func New(cfg MappingConfig, emit func(ctx context.Context, t ctelemetry.Telemetry) error) *Adapter {
	return &Adapter{cfg: cfg, emit: emit, state: adapters.StateConfigured}
}

func (a *Adapter) setState(s adapters.HealthState, msg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state, a.message = s, msg
}

func (a *Adapter) Connect(ctx context.Context) error {
	a.setState(adapters.StateConnecting, "")
	opts := paho.NewClientOptions().AddBroker(a.cfg.Broker).SetClientID("tide-mqtt")
	opts.ConnectTimeout = 5 * time.Second
	a.client = paho.NewClient(opts)
	// Exponential backoff + jitter, mandatory per §2.4.
	backoff := 500 * time.Millisecond
	for attempt := 0; ; attempt++ {
		if token := a.client.Connect(); token.WaitTimeout(6*time.Second) && token.Error() == nil {
			a.start = time.Now()
			a.setState(adapters.StateHealthy, "")
			return nil
		}
		select {
		case <-ctx.Done():
			a.setState(adapters.StateFailed, ctx.Err().Error())
			return ctx.Err()
		case <-time.After(backoff + time.Duration(rand.Int63n(int64(backoff)))):
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
		a.setState(adapters.StateReconnecting, fmt.Sprintf("attempt %d", attempt+1))
		if attempt > 8 {
			a.setState(adapters.StateFailed, "broker unreachable")
			return fmt.Errorf("mqtt: broker unreachable")
		}
	}
}

func (a *Adapter) Disconnect(_ context.Context) error {
	if a.client != nil && a.client.IsConnected() {
		a.client.Disconnect(500)
	}
	a.setState(adapters.StateConfigured, "")
	return nil
}

func (a *Adapter) Subscribe(ctx context.Context, handler adapters.TelemetryHandler) error {
	for _, topic := range a.cfg.Topics {
		t := topic
		token := a.client.Subscribe(t, 1, func(_ paho.Client, m paho.Message) {
			if err := handler(ctx, m.Topic(), m.Payload()); err == nil {
				a.mu.Lock()
				a.count++
				a.mu.Unlock()
			}
		})
		if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
			a.setState(adapters.StateDegraded, "subscribe failed: "+t)
			return fmt.Errorf("mqtt: subscribe %s: %v", t, token.Error())
		}
	}
	return nil
}

// HandleRaw maps + emits one raw payload (shared with tests/FleetSim boundary).
func (a *Adapter) HandleRaw(ctx context.Context, topic string, payload []byte) error {
	t, err := MapPayload(a.cfg, topic, payload, time.Now())
	if err != nil {
		return err
	}
	return a.emit(ctx, t)
}

func (a *Adapter) ListVehicles(_ context.Context) ([]adapters.Vehicle, error) {
	return nil, nil // generic MQTT has no device inventory
}

func (a *Adapter) GetVehicle(_ context.Context, _ string) (*adapters.Vehicle, error) {
	return nil, fmt.Errorf("mqtt: no device inventory")
}

func (a *Adapter) Health(_ context.Context) adapters.HealthStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var rate float64
	if !a.start.IsZero() {
		if el := time.Since(a.start).Seconds(); el > 0 {
			rate = float64(a.count) / el
		}
	}
	return adapters.HealthStatus{State: a.state, Message: a.message, MsgPerSec: rate}
}

func (a *Adapter) Capabilities() adapters.Capabilities {
	return adapters.Capabilities{LiveTelemetry: true}
}
