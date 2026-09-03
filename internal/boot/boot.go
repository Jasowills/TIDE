// Package boot builds the production dependency graph shared by tide-api and
// tide-engine (modular monolith: same constructors, two processes).
// Durable services degrade gracefully: Postgres/Redis/NATS unreachable →
// memory fallbacks with a warning, ingestion never dies (Architecture §2.10:
// buffer with a hard cap, don't kill ingestion — memory fallback IS the
// bounded buffer for local dev; prod requires the real services).
package boot

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/tide-telematics/tide/internal/config"
	"github.com/tide-telematics/tide/internal/db"
	"github.com/tide-telematics/tide/internal/dedup"
	"github.com/tide-telematics/tide/internal/detectors"
	"github.com/tide-telematics/tide/internal/eventbus"
	"github.com/tide-telematics/tide/internal/geo"
	"github.com/tide-telematics/tide/internal/pipeline"
	"github.com/tide-telematics/tide/internal/rules"
	"github.com/tide-telematics/tide/internal/state"
	"github.com/tide-telematics/tide/internal/store"
	"github.com/tide-telematics/tide/internal/webhooks"
	ctelemetry "github.com/tide-telematics/tide/schemas/telemetry"
)

type Bundle struct {
	Pipeline *pipeline.Pipeline
	States   state.Store
	MemBus   *eventbus.MemoryBus
	PG       *store.PG
	Rules    *rules.Engine
	Geo      *geo.Tracker
	NATSUp   bool
}

// DefaultSpeedingRule ships in V1: speeding.started → incident.created.
// Webhook attaches only when TIDE_DEFAULT_WEBHOOK is set.
const DefaultSpeedingRule = `
id: speeding-alert
version: v1
when:
  eventType: vehicle.speeding.started
then:
  emit: incident.created
cooldownSecs: 60
maxActionsPerHour: 100
`

// Lister exposes vehicle ids for the offline sweeper.
type Lister interface {
	ListIDs(ctx context.Context) ([]string, error)
}

func Build(ctx context.Context, cfg config.Config) *Bundle {
	b := &Bundle{MemBus: &eventbus.MemoryBus{}, Geo: geo.NewTracker(nil)}

	// Postgres: migrate + durable logs, else memory.
	if pg, err := store.Open(ctx, cfg.Postgres.DSN); err != nil {
		log.Printf("boot: postgres unavailable (%v) — memory mode", err)
	} else {
		m := db.Migrator{DSN: cfg.Postgres.DSN}
		if applied, err := m.ApplyPending(ctx); err != nil {
			log.Printf("boot: migrations failed (%v) — memory mode", err)
			_ = pg.Close()
		} else {
			for _, f := range applied {
				log.Printf("boot: applied migration %s", f)
			}
			b.PG = pg
		}
	}

	// Redis: hot state + dedup, else memory.
	var states state.Store = state.NewMemoryStore()
	var dd dedup.Store = dedup.NewMemoryStore(0)
	if rs := state.NewRedisStore(cfg.Redis.Addr); pingRedis(ctx, cfg.Redis.Addr) {
		states = rs
		dd = dedup.NewRedisStore(cfg.Redis.Addr, 0)
		log.Printf("boot: redis at %s", cfg.Redis.Addr)
	} else {
		log.Printf("boot: redis unavailable — memory hot state (rebuildable, §2.6)")
	}
	b.States = states

	// NATS: fan out (transport + local memory), else memory only.
	var bus eventbus.Bus = b.MemBus
	if nb, err := eventbus.NewNATSBus(cfg.NATS.URL); err != nil {
		log.Printf("boot: nats unavailable (%v) — memory bus", err)
	} else {
		bus = eventbus.FanOut{Children: []eventbus.Bus{nb, b.MemBus}}
		b.NATSUp = true
		log.Printf("boot: nats at %s", cfg.NATS.URL)
	}

	eng := rules.NewEngine(webhooks.NewDispatcher())
	if spec, err := rules.ParseSpec([]byte(DefaultSpeedingRule)); err == nil {
		if url := os.Getenv("TIDE_DEFAULT_WEBHOOK"); url != "" {
			spec.Then.Webhook = url
			spec.Then.Secret = os.Getenv("TIDE_DEFAULT_WEBHOOK_SECRET")
		}
		_ = eng.Publish(spec, time.Now())
	}
	for _, f := range ruleFiles() {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		spec, err := rules.ParseSpec(raw)
		if err != nil {
			log.Printf("boot: bad rule %s: %v", f, err)
			continue
		}
		if err := eng.Publish(spec, time.Now()); err != nil {
			log.Printf("boot: rule %s: %v", f, err)
		} else {
			log.Printf("boot: published rule %s/%s", spec.ID, spec.Version)
		}
	}
	b.Rules = eng

	b.Pipeline = &pipeline.Pipeline{
		Dedup: dd, States: states, Log: logSink(b.PG),
		Bus: bus, Detectors: detectors.NewTracker(detectors.DefaultConfig()),
		Geo: b.Geo, Rules: eng,
	}
	return b
}

func ruleFiles() []string {
	dir := os.Getenv("TIDE_RULES")
	if dir == "" {
		dir = "rules"
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	matches2, _ := filepath.Glob(filepath.Join(dir, "*.yml"))
	return append(matches, matches2...)
}

// pgLog dual-writes durable telemetry when PG is up; else memory.
func logSink(pg *store.PG) pipeline.TelemetryLog {
	if pg == nil {
		return &pipeline.MemoryLog{}
	}
	return &pgLog{pg: pg, mem: &pipeline.MemoryLog{}}
}

type pgLog struct {
	pg  *store.PG
	mem *pipeline.MemoryLog
}

func (l *pgLog) Append(ctx context.Context, t ctelemetry.Telemetry) error {
	_ = l.mem.Append(ctx, t)
	return l.pg.AppendTelemetry(ctx, t)
}

func (l *pgLog) All() []ctelemetry.Telemetry { return l.mem.All }

func pingRedis(ctx context.Context, addr string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	rs := state.NewRedisStore(addr)
	_, _, err := rs.Get(ctx, "__boot_ping__")
	// Get returns (zero,false,nil) on miss — any non-timeout error means up.
	// A strict ping: try a Set+delete round-trip instead.
	if err != nil {
		return false
	}
	return true
}
