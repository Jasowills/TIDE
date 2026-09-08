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
	"sync"
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
	NATSBus  *eventbus.ResilientBus // nil unless NATS reachable at boot
}

// newDispatcher honors TIDE_WEBHOOK_ALLOW_PRIVATE=1 for dev/test loops that
// point webhooks at loopback. Production default is deny-private (A01).
func newDispatcher() *webhooks.Dispatcher {
	d := webhooks.NewDispatcher()
	if os.Getenv("TIDE_WEBHOOK_ALLOW_PRIVATE") == "1" {
		d.AllowPrivate = true
	}
	return d
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

	// Redis: hot state + dedup, else memory. Redis-backed stores ride behind
	// circuit breakers so a mid-run outage degrades to memory instead of
	// 500ing ingestion (ADV-0003); recovery flips back automatically.
	var states state.Store = state.NewMemoryStore()
	var dd dedup.Store = dedup.NewMemoryStore(0)
	if rs := state.NewRedisStore(cfg.Redis.Addr); pingRedis(ctx, cfg.Redis.Addr) {
		states = state.NewFallback(rs, state.NewMemoryStore())
		dd = dedup.NewFallback(dedup.NewRedisStore(cfg.Redis.Addr, 0), dedup.NewMemoryStore(0))
		log.Printf("boot: redis at %s", cfg.Redis.Addr)
	} else {
		log.Printf("boot: redis unavailable — memory hot state (rebuildable, §2.6)")
	}
	b.States = states

	// NATS: fan out (transport + local memory), else memory only. The NATS
	// leg is wrapped for redial so an outage ends on recovery, not restart.
	var bus eventbus.Bus = b.MemBus
	if nb, err := eventbus.NewNATSBus(cfg.NATS.URL); err != nil {
		log.Printf("boot: nats unavailable (%v) — memory bus", err)
	} else {
		rb := eventbus.NewResilientBus(func() (eventbus.Bus, error) {
			return eventbus.NewNATSBus(cfg.NATS.URL)
		}, nb)
		b.NATSBus = rb
		bus = eventbus.FanOut{Children: []eventbus.Bus{rb, b.MemBus}}
		b.NATSUp = true
		log.Printf("boot: nats at %s", cfg.NATS.URL)
	}

	eng := rules.NewEngine(newDispatcher())
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
	// Webhook workers: async delivery off the ingest path (ADV-0006).
	go eng.Dispatcher.Run(ctx, webhookWorkers)

	b.Pipeline = &pipeline.Pipeline{
		Dedup: dd, States: states, Log: logSink(ctx, b.PG),
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

// webhookWorkers bounds concurrent downstream deliveries.
const webhookWorkers = 2

// telemetryStore is the durable write path (*store.PG in prod; stubbed in tests).
type telemetryStore interface {
	AppendTelemetry(ctx context.Context, t ctelemetry.Telemetry) error
}

// logSink builds the durable write path: direct PG writes, bounded pending
// queue, background drain. Callers get a TelemetryLog; the drain loop starts here.
func logSink(ctx context.Context, pg *store.PG) pipeline.TelemetryLog {
	if pg == nil {
		return &pipeline.MemoryLog{}
	}
	l := &pgLog{pg: pg}
	l.StartDrain(ctx)
	return l
}

// pgBufferCap bounds the outage buffer (§2.10: "buffer with a hard cap").
// Past it the oldest points are shed (newest-wins: live state matters more
// than history; PG upserts keep redelivery safe). Memory can never grow
// without limit (ADV-0004).
const (
	pgBufferCap     = 10000
	pgDrainInterval = 10 * time.Second
	pgDrainBatch    = 500
)

// pgLog dual-writes durable telemetry when PG is up. When PG appends fail,
// points park in a bounded pending queue drained by StartDrain — nothing is
// silently dropped on recovery, and memory cannot grow without bound.
type pgLog struct {
	pg      telemetryStore
	mu      sync.Mutex
	pending []ctelemetry.Telemetry
	dropped int64
}

func (l *pgLog) Append(ctx context.Context, t ctelemetry.Telemetry) error {
	if err := l.pg.AppendTelemetry(ctx, t); err == nil {
		return nil
	} else {
		_ = err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.pending) >= pgBufferCap {
		l.dropped++
		log.Printf("boot: telemetry buffer full (%d), shedding oldest", pgBufferCap)
		// Shed oldest, keep newest: bounded memory matters more than any
		// single point; onConflict upserts keep redelivery safe.
		copy(l.pending, l.pending[1:])
		l.pending = l.pending[:len(l.pending)-1]
	}
	l.pending = append(l.pending, t)
	log.Printf("boot: postgres append failed, buffered in memory (%d pending)", len(l.pending))
	return nil
}

// StartDrain retries the pending queue against PG until ctx ends: what was
// accepted during an outage reaches durable storage after recovery (ADV-0004).
// PG upserts make redelivery idempotent; a failed batch stays queued.
func (l *pgLog) StartDrain(ctx context.Context) {
	go func() {
		t := time.NewTicker(pgDrainInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				l.drainOnce(ctx)
			}
		}
	}()
}

func (l *pgLog) drainOnce(ctx context.Context) {
	l.mu.Lock()
	n := len(l.pending)
	if n > pgDrainBatch {
		n = pgDrainBatch
	}
	batch := append([]ctelemetry.Telemetry(nil), l.pending[:n]...)
	l.mu.Unlock()
	flushed := 0
	for _, t := range batch {
		if err := l.pg.AppendTelemetry(ctx, t); err != nil {
			break
		}
		flushed++
	}
	if flushed == 0 {
		return
	}
	l.mu.Lock()
	l.pending = append([]ctelemetry.Telemetry(nil), l.pending[flushed:]...)
	l.mu.Unlock()
	log.Printf("boot: drained %d buffered points to postgres", flushed)
}

// Pending reports buffered (not-yet-durable) points; Dropped counts shed points.
func (l *pgLog) Pending() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.pending)
}

func (l *pgLog) Dropped() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.dropped
}

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
