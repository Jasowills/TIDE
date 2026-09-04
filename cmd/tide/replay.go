package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tide-telematics/tide/internal/dedup"
	"github.com/tide-telematics/tide/internal/detectors"
	"github.com/tide-telematics/tide/internal/eventbus"
	"github.com/tide-telematics/tide/internal/pipeline"
	"github.com/tide-telematics/tide/internal/replay"
	"github.com/tide-telematics/tide/internal/rules"
	"github.com/tide-telematics/tide/internal/state"
	"github.com/tide-telematics/tide/simulator/faults"
	"github.com/tide-telematics/tide/simulator/generators"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

// tide replay --scenario speeding --vehicles 5 --rule rules/a.yaml [--compare rules/b.yaml]
// Replays through the production pipeline (never a parallel implementation).
func runReplay(args []string) {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	scenario := fs.String("scenario", "speeding", "scenario name (or --input file)")
	vehicles := fs.Int("vehicles", 5, "vehicles (scenario mode)")
	seed := fs.Int64("seed", 42, "seed")
	rule := fs.String("rule", "", "rule YAML to pin (repeatable)")
	compare := fs.String("compare", "", "second rule YAML for v1-vs-v2 diff")
	input := fs.String("input", "", "recorded telemetry JSON file (overrides scenario)")
	scenDir := fs.String("scenarios", "simulator/scenarios", "scenario dir")
	_ = fs.Parse(args)

	var pts []ctelemetry.Telemetry
	if *input != "" {
		raw, err := os.ReadFile(*input)
		if err != nil {
			fatalf("replay: %v", err)
		}
		if err := json.Unmarshal(raw, &pts); err != nil {
			fatalf("replay: bad input: %v", err)
		}
	} else {
		raw, err := os.ReadFile(filepath.Join(*scenDir, *scenario+".yaml"))
		if err != nil {
			fatalf("replay: unknown scenario %q", *scenario)
		}
		scen, err := generators.LoadScenario(raw)
		if err != nil {
			fatalf("replay: %v", err)
		}
		pts = generators.Generate(scen, *vehicles, *seed, "replay", time.Now().UTC())
		pts = faults.Apply(pts, faults.Config{}, *seed)
	}

	build := func(rulePath string) func() *pipeline.Pipeline {
		return func() *pipeline.Pipeline {
			p, _, _ := pipeline.NewTestPipeline()
			if rulePath != "" {
				raw, err := os.ReadFile(rulePath)
				if err != nil {
					fatalf("replay: rule: %v", err)
				}
				spec, err := rules.ParseSpec(raw)
				if err != nil {
					fatalf("replay: rule: %v", err)
				}
				eng := rules.NewEngine(nil)
				if err := eng.Publish(spec, time.Now()); err != nil {
					fatalf("replay: rule: %v", err)
				}
				p.Rules = eng
			}
			return p
		}
	}

	ctx := context.Background()
	if *compare != "" {
		a, b, err := replay.Compare(ctx, pts, build(*rule), build(*compare))
		if err != nil {
			fatalf("replay: %v", err)
		}
		fmt.Printf("rule A (%s): %d events\n", *rule, len(a))
		for _, t := range a {
			fmt.Println("  A", t)
		}
		fmt.Printf("rule B (%s): %d events\n", *compare, len(b))
		for _, t := range b {
			fmt.Println("  B", t)
		}
		return
	}
	res, err := replay.Run(ctx, pts, build(*rule))
	if err != nil {
		fatalf("replay: %v", err)
	}
	// Determinism self-check: run twice, compare ids.
	res2, err := replay.Run(ctx, pts, build(*rule))
	if err != nil {
		fatalf("replay: %v", err)
	}
	deterministic := len(res.Events) == len(res2.Events)
	if deterministic {
		for i := range res.Events {
			if res.Events[i].ID != res2.Events[i].ID {
				deterministic = false
				break
			}
		}
	}
	fmt.Printf("replay: %d points → %d events (deterministic=%v)\n", res.Points, len(res.Events), deterministic)
	for _, e := range res.Events {
		fmt.Printf("  %s %s corr=%s rule=%s/%s\n", e.Timestamp.Format(time.RFC3339), e.Type, e.CorrelationID, e.RuleID, e.RuleVersion)
	}
	if !deterministic {
		os.Exit(1)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// silence unused imports if detectors/state evolve
var _ = detectors.DefaultConfig
var _ = state.MotionMoving
var _ = dedup.NewMemoryStore
var _ = eventbus.MemoryBus{}
