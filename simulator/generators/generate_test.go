package generators

import (
	"testing"
	"time"
)

func TestDeterministic(t *testing.T) {
	s := Scenario{Seed: 42, StepSecs: 5, DurationSecs: 60, Mix: map[string]float64{"mixed": 1}}
	s.Start.Lat, s.Start.Lng = 52.5, 13.4
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := Generate(s, 5, 42, "t", base)
	b := Generate(s, 5, 42, "t", base)
	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d %d", len(a), len(b))
	}
	for i := range a {
		if !a[i].Timestamp.Equal(b[i].Timestamp) || a[i].Location != b[i].Location {
			t.Fatalf("non-deterministic point %d", i)
		}
		if err := a[i].Validate(); err != nil {
			t.Fatalf("invalid generated point: %v", err)
		}
	}
}

func TestSpeedingScenarioFast(t *testing.T) {
	s := Scenario{Seed: 1, StepSecs: 5, DurationSecs: 120, Mix: map[string]float64{"speeding": 1}}
	s.Start.Lat, s.Start.Lng = 52.5, 13.4
	pts := Generate(s, 3, 1, "t", time.Now())
	fast := 0
	for _, p := range pts {
		if p.SpeedKmh != nil && *p.SpeedKmh > 100 {
			fast++
		}
	}
	if fast == 0 {
		t.Fatal("speeding scenario produced no fast points")
	}
}
