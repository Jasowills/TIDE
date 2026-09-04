package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tide-telematics/tide/internal/observability"
	"github.com/tide-telematics/tide/simulator/faults"
	"github.com/tide-telematics/tide/simulator/generators"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

type simFlags struct {
	vehicles       int
	scenario       string
	rate           int
	seed           int64
	api            string
	duplicate      float64
	late           float64
	missing        float64
	outOfOrder     float64
	gpsDrift       float64
	offlineVehs    float64
}

func runSimulate(args []string) {
	fs := flag.NewFlagSet("simulate", flag.ExitOnError)
	f := simFlags{}
	fs.IntVar(&f.vehicles, "vehicles", 10, "number of simulated vehicles")
	fs.StringVar(&f.scenario, "scenario", "mixed", "scenario name")
	fs.IntVar(&f.rate, "rate", 0, "max points/sec (0 = as fast as possible)")
	fs.Int64Var(&f.seed, "seed", 0, "random seed (0 = scenario default)")
	fs.StringVar(&f.api, "api", "http://localhost:8080", "tide-api base URL")
	fs.Float64Var(&f.duplicate, "duplicate-events", 0, "duplicate injection rate 0-1")
	fs.Float64Var(&f.late, "late-events", 0, "late injection rate 0-1")
	fs.Float64Var(&f.missing, "missing-events", 0, "drop rate 0-1")
	fs.Float64Var(&f.outOfOrder, "out-of-order", 0, "reorder rate 0-1")
	fs.Float64Var(&f.gpsDrift, "gps-drift", 0, "GPS drift meters (stddev)")
	fs.Float64Var(&f.offlineVehs, "offline-vehicles", 0, "fraction of vehicles to black out")
	scenDir := fs.String("scenarios", os.Getenv("TIDE_SCENARIOS"), "scenario dir (default ./simulator/scenarios)")
	_ = fs.Parse(args)
	if *scenDir == "" {
		*scenDir = "simulator/scenarios"
	}

	raw, err := os.ReadFile(filepath.Join(*scenDir, f.scenario+".yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "simulate: unknown scenario %q in %s\n", f.scenario, *scenDir)
		os.Exit(1)
	}
	scen, err := generators.LoadScenario(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "simulate: %v\n", err)
		os.Exit(1)
	}
	pts := generators.Generate(scen, f.vehicles, f.seed, "default", time.Now().UTC())
	pts = faults.Apply(pts, faults.Config{
		DuplicateRate: f.duplicate, LateRate: f.late, MissingRate: f.missing,
		OutOfOrderRate: f.outOfOrder, GPSDriftM: f.gpsDrift, OfflineVehicles: f.offlineVehs,
	}, f.seed)
	ctx := context.Background()
	shutdown, err := observability.Init(ctx, "tide-sim")
	if err != nil {
		fmt.Fprintf(os.Stderr, "otel: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = shutdown(ctx) }()
	observability.DummySpan(ctx)

	sent, err := postBatches(f.api, pts, f.rate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "simulate: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("simulate: scenario=%s vehicles=%d points=%d sent=%d → %s\n",
		f.scenario, f.vehicles, len(pts), sent, f.api)
}

func postBatches(api string, pts []ctelemetry.Telemetry, rate int) (int, error) {
	const chunk = 100
	sent := 0
	var throttle <-chan time.Time
	if rate > 0 {
		t := time.NewTicker(time.Second / time.Duration(rate))
		defer t.Stop()
		throttle = t.C
	}
	for i := 0; i < len(pts); i += chunk {
		end := i + chunk
		if end > len(pts) {
			end = len(pts)
		}
		body, _ := json.Marshal(map[string]any{"batch": pts[i:end]})
		resp, err := http.Post(api+"/v1/telemetry:batch", "application/json", bytes.NewReader(body))
		if err != nil {
			return sent, fmt.Errorf("post batch: %w (is tide-api running at %s?)", err, api)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			return sent, fmt.Errorf("post batch: api returned %d", resp.StatusCode)
		}
		sent += end - i
		if throttle != nil {
			for j := i; j < end; j++ {
				<-throttle
			}
		}
	}
	return sent, nil
}
