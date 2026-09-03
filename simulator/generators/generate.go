// Package generators produces deterministic simulated telemetry (FleetSim).
// Same scenario + seed → identical points, so replay/rule tests are credible
// without hardware (Architecture §2.9). Output is canonical telemetry,
// indistinguishable at the ingestion boundary from a real adapter.
package generators

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
	"gopkg.in/yaml.v3"
)

type Scenario struct {
	Name       string             `yaml:"name"`
	Seed       int64              `yaml:"seed"`
	StepSecs   int                `yaml:"stepSecs"`
	DurationSecs int              `yaml:"durationSecs"`
	Start      struct {
		Lat float64 `yaml:"lat"`
		Lng float64 `yaml:"lng"`
	} `yaml:"start"`
	SpreadKm float64            `yaml:"spreadKm"`
	Mix      map[string]float64 `yaml:"mix"`
}

func LoadScenario(data []byte) (Scenario, error) {
	var s Scenario
	if err := yaml.Unmarshal(data, &s); err != nil {
		return Scenario{}, fmt.Errorf("sim: parse scenario: %w", err)
	}
	if s.StepSecs <= 0 || s.DurationSecs <= 0 {
		return Scenario{}, fmt.Errorf("sim: stepSecs + durationSecs required")
	}
	if len(s.Mix) == 0 {
		return Scenario{}, fmt.Errorf("sim: mix required")
	}
	return s, nil
}

func pickProfile(r *rand.Rand, mix map[string]float64) string {
	x := r.Float64()
	acc := 0.0
	// Deterministic order: fixed profile list.
	for _, name := range []string{"normal", "speeding", "idling", "offline", "mixed"} {
		w := mix[name]
		acc += w
		if x < acc {
			return name
		}
	}
	return "normal"
}

// speedFor returns (speedKmh, ignition) for a profile at step i.
func speedFor(profile string, r *rand.Rand, i int) (float64, bool) {
	switch profile {
	case "speeding":
		if i%6 < 4 { // sustained bursts over common limits
			return 110 + r.Float64()*30, true
		}
		return 45 + r.Float64()*10, true
	case "idling":
		return 0, true
	case "offline":
		return 0, false
	default: // normal urban driving
		return 20 + r.Float64()*40, true
	}
}

// Generate emits one point per step per vehicle. Deterministic in (scenario, seed).
func Generate(s Scenario, vehicles int, seed int64, tenant string, base time.Time) []ctelemetry.Telemetry {
	if seed == 0 {
		seed = s.Seed
	}
	r := rand.New(rand.NewSource(seed))
	steps := s.DurationSecs / s.StepSecs
	var out []ctelemetry.Telemetry
	for v := 0; v < vehicles; v++ {
		profile := pickProfile(r, s.Mix)
		lat := s.Start.Lat + (r.Float64()-0.5)*s.SpreadKm/111.0
		lng := s.Start.Lng + (r.Float64()-0.5)*s.SpreadKm/85.0
		heading := r.Float64() * 360
		vid := fmt.Sprintf("sim-%03d", v)
		var seq int64
		for i := 0; i < steps; i++ {
			ts := base.Add(time.Duration(i*s.StepSecs) * time.Second)
			if profile == "offline" && i > steps/3 {
				break // vehicle goes dark a third in
			}
			speed, ign := speedFor(profile, r, i)
			// Advance position by speed*step along heading (flat-earth, fine for sim).
			distKm := speed * float64(s.StepSecs) / 3600.0
			heading += (r.Float64() - 0.5) * 20
			lat += distKm * math.Cos(heading*math.Pi/180) / 111.0
			lng += distKm * math.Sin(heading*math.Pi/180) / 85.0
			seq++
			sp, ig := speed, ign
			sq := seq // fresh copy per point: shared address would alias all sequences
			out = append(out, ctelemetry.Telemetry{
				ID: fmt.Sprintf("%s-%d", vid, seq), TenantID: tenant,
				VehicleID: vid, DeviceID: vid,
				Timestamp: ts, ReceivedAt: ts,
				Location:  ctelemetry.Location{Lat: lat, Lng: lng},
				SpeedKmh:  &sp, Ignition: &ig,
				Raw:       map[string]any{"sim": true, "profile": profile},
				Source:    ctelemetry.Source{Provider: "fleetsim", Protocol: "sim", DeviceID: vid},
				Metadata: ctelemetry.Metadata{CorrelationID: fmt.Sprintf("sim-%d", seed),
					SchemaVersion: ctelemetry.CurrentSchemaVersion, Sequence: &sq, Quality: "good"},
			})
		}
	}
	return out
}
