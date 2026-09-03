// Package replay replays recorded telemetry through the SAME production
// processing path (Architecture §2.8, T070). There is no separate replay
// engine: Replay builds a fresh Pipeline (fresh dedup/state/detectors, rule
// versions pinned by the caller) and calls Process per point in event-time
// order. Determinism requires pinning telemetry, config, rule version,
// schema and algorithm versions — all are caller inputs, never live data.
package replay

import (
	"context"
	"fmt"
	"sort"

	"github.com/tide-telematics/tide/internal/pipeline"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
	"github.com/tide-telematics/tide/schemas/events"
)

// Result is one replay run.
type Result struct {
	Events []events.Event
	Points int
}

// Run replays points through a fresh pipeline from build().
func Run(ctx context.Context, points []ctelemetry.Telemetry, build func() *pipeline.Pipeline) (Result, error) {
	ordered := append([]ctelemetry.Telemetry{}, points...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Timestamp.Equal(ordered[j].Timestamp) {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].Timestamp.Before(ordered[j].Timestamp)
	})
	p := build()
	var out []events.Event
	for _, pt := range ordered {
		evs, err := p.Process(ctx, pt)
		if err != nil {
			return Result{}, fmt.Errorf("replay: process %s: %w", pt.ID, err)
		}
		out = append(out, evs...)
	}
	return Result{Events: out, Points: len(ordered)}, nil
}

func eventTypes(evs []events.Event) []string {
	var out []string
	for _, e := range evs {
		out = append(out, e.Type)
	}
	return out
}

// Compare runs the same window under two pipeline builds (e.g. rule v1 vs
// v2) and returns both type sequences for diffing (T072).
func Compare(ctx context.Context, points []ctelemetry.Telemetry, a, b func() *pipeline.Pipeline) ([]string, []string, error) {
	ra, err := Run(ctx, points, a)
	if err != nil {
		return nil, nil, err
	}
	rb, err := Run(ctx, points, b)
	if err != nil {
		return nil, nil, err
	}
	return eventTypes(ra.Events), eventTypes(rb.Events), nil
}
