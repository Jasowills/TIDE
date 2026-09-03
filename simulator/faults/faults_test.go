package faults

import (
	"testing"
	"time"

	"github.com/tide-telematics/tide/simulator/generators"
)

func TestFaultsDeterministicAndEffective(t *testing.T) {
	s := generators.Scenario{Seed: 5, StepSecs: 5, DurationSecs: 120, Mix: map[string]float64{"normal": 1}}
	s.Start.Lat, s.Start.Lng = 52.5, 13.4
	base := time.Now()
	pts := generators.Generate(s, 4, 5, "t", base)
	cfg := Config{DuplicateRate: 0.2, MissingRate: 0.1, OutOfOrderRate: 0.2, GPSDriftM: 50}
	a := Apply(pts, cfg, 9)
	b := Apply(pts, cfg, 9)
	if len(a) != len(b) {
		t.Fatalf("faults non-deterministic: %d vs %d", len(a), len(b))
	}
	dups := 0
	for _, p := range a {
		if len(p.ID) > 4 && p.ID[len(p.ID)-4:] == "-dup" {
			dups++
		}
		if err := p.Validate(); err != nil {
			t.Fatalf("faulted point invalid: %v (id %s)", err, p.ID)
		}
	}
	if dups == 0 {
		t.Fatal("duplicate fault injected nothing")
	}
}
