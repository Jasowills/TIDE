// Package faults injects production-grade mess (Architecture §2.9, T082):
// duplicates, late/out-of-order/missing points, GPS drift, offline vehicles.
// Deterministic in seed — same faults every run, so tests are reproducible.
package faults

import (
	"math/rand"
	"time"

	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

type Config struct {
	DuplicateRate   float64 `yaml:"duplicateRate"`
	LateRate        float64 `yaml:"lateRate"`
	LateBy          string  `yaml:"lateBy"`
	MissingRate     float64 `yaml:"missingRate"`
	OutOfOrderRate  float64 `yaml:"outOfOrderRate"`
	GPSDriftM       float64 `yaml:"gpsDriftM"`
	OfflineVehicles float64 `yaml:"offlineVehicles"`
}

func (c Config) lateDuration() time.Duration {
	if c.LateBy == "" {
		return 2 * time.Minute
	}
	d, err := time.ParseDuration(c.LateBy)
	if err != nil {
		return 2 * time.Minute
	}
	return d
}

// Apply injects faults into points deterministically.
func Apply(points []ctelemetry.Telemetry, cfg Config, seed int64) []ctelemetry.Telemetry {
	r := rand.New(rand.NewSource(seed + 1))
	// Offline vehicles: drop whole vehicles first.
	var kept []ctelemetry.Telemetry
	if cfg.OfflineVehicles > 0 {
		byV := map[string][]ctelemetry.Telemetry{}
		var order []string
		for _, p := range points {
			if _, ok := byV[p.VehicleID]; !ok {
				order = append(order, p.VehicleID)
			}
			byV[p.VehicleID] = append(byV[p.VehicleID], p)
		}
		for _, vid := range order {
			if r.Float64() < cfg.OfflineVehicles {
				continue
			}
			kept = append(kept, byV[vid]...)
		}
	} else {
		kept = points
	}
	var out []ctelemetry.Telemetry
	for _, p := range kept {
		if cfg.MissingRate > 0 && r.Float64() < cfg.MissingRate {
			continue
		}
		q := p
		if cfg.GPSDriftM > 0 {
			// ~1deg lat ≈ 111km.
			q.Location.Lat += (r.NormFloat64() * cfg.GPSDriftM) / 111000.0
			q.Location.Lng += (r.NormFloat64() * cfg.GPSDriftM) / 85000.0
		}
		if cfg.LateRate > 0 && r.Float64() < cfg.LateRate {
			q.Timestamp = q.Timestamp.Add(-cfg.lateDuration())
			q.Metadata.Quality = "degraded:late-injected"
		}
		out = append(out, q)
		if cfg.DuplicateRate > 0 && r.Float64() < cfg.DuplicateRate {
			dup := q
			dup.ID = q.ID + "-dup" // same sequence → dedup hit at ingestion
			out = append(out, dup)
		}
	}
	// Out-of-order: swap adjacent pairs with probability.
	for i := 0; i+1 < len(out); i++ {
		if cfg.OutOfOrderRate > 0 && r.Float64() < cfg.OutOfOrderRate {
			out[i], out[i+1] = out[i+1], out[i]
			i++
		}
	}
	return out
}
