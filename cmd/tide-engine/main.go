package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/tide-telematics/tide/internal/config"
)

func main() {
	cfgPath := flag.String("config", os.Getenv("TIDE_CONFIG"), "path to tide.yaml (or empty)")
	flag.Parse()
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("tide-engine: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := runEngine(ctx, cfg); err != nil {
		log.Fatalf("tide-engine: %v", err)
	}
}
