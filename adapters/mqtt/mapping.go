// Package mqtt is the generic MQTT adapter (T021): a YAML field-mapping maps
// ANY device payload to canonical telemetry with zero Go code.
package mqtt

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
	"gopkg.in/yaml.v3"
)

func newCorrID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// MappingConfig is loaded from YAML. Example:
//
//	broker: tcp://localhost:1883
//	topics: ["fleet/+/telemetry"]
//	defaults: {tenant: t1, provider: mytracker, protocol: mqtt}
//	fields: {vehicleId: device.id, lat: gps.lat, lng: gps.lon, speed: gps.speed}
//	speedUnit: kmh|mph|ms   (default kmh)
//	topicVehicleIndex: 1    # fleet/<vehicle>/telemetry → segment used as vehicle id
type MappingConfig struct {
	Broker            string            `yaml:"broker"`
	Topics            []string          `yaml:"topics"`
	Defaults          map[string]string `yaml:"defaults"`
	Fields            map[string]string `yaml:"fields"`
	SpeedUnit         string            `yaml:"speedUnit"`
	TopicVehicleIndex *int              `yaml:"topicVehicleIndex"`
}

func LoadMapping(data []byte) (MappingConfig, error) {
	var c MappingConfig
	if err := yaml.Unmarshal(data, &c); err != nil {
		return MappingConfig{}, fmt.Errorf("mqtt: parse mapping: %w", err)
	}
	if c.Broker == "" || len(c.Topics) == 0 {
		return MappingConfig{}, fmt.Errorf("mqtt: mapping requires broker + topics")
	}
	switch c.SpeedUnit {
	case "", "kmh", "mph", "ms":
	default:
		return MappingConfig{}, fmt.Errorf("mqtt: unknown speedUnit %q", c.SpeedUnit)
	}
	return c, nil
}

func lookup(doc map[string]any, path string) (any, bool) {
	var cur any = doc
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[seg]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	}
	return 0, false
}

func toString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(x), true
	}
	return "", false
}

// MapPayload converts one raw MQTT JSON payload to canonical telemetry.
// Units convert here (adapter responsibility, never core — §2.3).
func MapPayload(cfg MappingConfig, topic string, payload []byte, now time.Time) (ctelemetry.Telemetry, error) {
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		return ctelemetry.Telemetry{}, fmt.Errorf("mqtt: invalid JSON: %w", err)
	}
	get := func(name string) (any, bool) {
		p, ok := cfg.Fields[name]
		if !ok {
			return nil, false
		}
		return lookup(doc, p)
	}
	t := ctelemetry.Telemetry{
		TenantID:   cfg.Defaults["tenant"],
		ReceivedAt: now.UTC(),
		Raw:        doc,
		Source:     ctelemetry.Source{Provider: cfg.Defaults["provider"], Protocol: "mqtt"},
		Metadata: ctelemetry.Metadata{
			CorrelationID: newCorrID(),
			SchemaVersion: ctelemetry.CurrentSchemaVersion,
			Quality:       "good",
		},
	}
	if t.Source.Provider == "" {
		t.Source.Provider = "mqtt"
	}
	if v, ok := get("vehicleId"); ok {
		if s, ok := toString(v); ok {
			t.VehicleID = s
			t.DeviceID = s
		}
	}
	if cfg.TopicVehicleIndex != nil && t.VehicleID == "" {
		segs := strings.Split(topic, "/")
		if *cfg.TopicVehicleIndex < len(segs) {
			t.VehicleID = segs[*cfg.TopicVehicleIndex]
			t.DeviceID = t.VehicleID
		}
	}
	if v, ok := get("lat"); ok {
		if f, ok := toFloat(v); ok {
			t.Location.Lat = f
		}
	}
	if v, ok := get("lng"); ok {
		if f, ok := toFloat(v); ok {
			t.Location.Lng = f
		}
	}
	if v, ok := get("speed"); ok {
		if f, ok := toFloat(v); ok {
			switch cfg.SpeedUnit {
			case "mph":
				f *= 1.60934
			case "ms":
				f *= 3.6
			}
			t.SpeedKmh = &f
		}
	}
	if v, ok := get("ignition"); ok {
		switch x := v.(type) {
		case bool:
			t.Ignition = &x
		case string:
			b := x == "true" || x == "1" || x == "on"
			t.Ignition = &b
		case float64:
			b := x != 0
			t.Ignition = &b
		}
	}
	if v, ok := get("timestamp"); ok {
		if s, ok := toString(v); ok {
			for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05"} {
				if ts, err := time.Parse(layout, s); err == nil {
					t.Timestamp = ts.UTC()
					break
				}
			}
		}
		if f, ok := toFloat(v); ok && t.Timestamp.IsZero() && f > 1e9 {
			t.Timestamp = time.Unix(int64(f), 0).UTC() // epoch seconds
		}
	}
	if t.Timestamp.IsZero() {
		t.Timestamp = now.UTC()
		t.Metadata.Quality = "degraded:no-timestamp"
	}
	if v, ok := get("sequence"); ok {
		if f, ok := toFloat(v); ok {
			s := int64(f)
			t.Metadata.Sequence = &s
		}
	}
	t.Source.DeviceID = t.DeviceID
	if t.TenantID == "" || t.VehicleID == "" {
		return ctelemetry.Telemetry{}, fmt.Errorf("mqtt: mapping must yield tenant + vehicleId")
	}
	if err := t.Validate(); err != nil {
		return ctelemetry.Telemetry{}, fmt.Errorf("mqtt: mapped payload invalid: %w", err)
	}
	return t, nil
}
