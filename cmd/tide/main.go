package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/tide-telematics/tide/internal/config"
	"github.com/tide-telematics/tide/internal/db"
	"github.com/tide-telematics/tide/internal/health"
	"github.com/tide-telematics/tide/internal/observability"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "doctor":
		doctor(os.Args[2:])
	case "simulate":
		simulate(os.Args[2:])
	case "replay":
		runReplay(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: tide <doctor|simulate|replay> [flags]")
	fmt.Fprintln(os.Stderr, "  doctor    --config tide.yaml   check postgres/redis/nats + schema version")
	fmt.Fprintln(os.Stderr, "  simulate  --vehicles N --scenario mixed   (full FleetSim lands in Phase 8; Phase 0 emits a connection probe)")
}

func loadCfg(args []string) config.Config {
	fs := flag.NewFlagSet("cfg", flag.ContinueOnError)
	cfgPath := fs.String("config", os.Getenv("TIDE_CONFIG"), "path to tide.yaml (or empty for defaults+env)")
	_ = fs.Parse(args)
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tide: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

func doctor(args []string) {
	cfg := loadCfg(args)
	ctx := context.Background()
	shutdown, err := observability.Init(ctx, "tide-doctor")
	if err != nil {
		fmt.Fprintf(os.Stderr, "otel: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = shutdown(ctx) }()
	observability.DummySpan(ctx) // T006: proves trace pipeline is wired

	pg := health.CheckPostgres(ctx, cfg.Postgres.DSN)
	rd := health.CheckRedis(ctx, cfg.Redis.Addr)
	nc := health.CheckNATS(cfg.NATS.URL)

	ver, verr := db.Migrator{DSN: cfg.Postgres.DSN}.Version(ctx)
	pending, perr := db.Migrator{DSN: cfg.Postgres.DSN}.Pending(ctx)

	failed := false
	for _, s := range []health.Status{pg, rd, nc} {
		state := "ok"
		if !s.OK {
			state = "FAIL"
			failed = true
		}
		fmt.Printf("%-10s %s (%s)\n", s.Name, state, s.Detail)
	}
	if verr != nil || perr != nil {
		fmt.Printf("schema     FAIL (version query: %v pending query: %v)\n", verr, perr)
		failed = true
	} else {
		fmt.Printf("schema     version=%d pending=%d\n", ver, len(pending))
		for _, p := range pending {
			fmt.Printf("  pending: %s\n", p)
		}
	}
	if failed {
		os.Exit(1)
	}
}

func simulate(args []string) {
	runSimulate(args)
}
