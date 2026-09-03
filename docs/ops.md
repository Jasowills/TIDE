# Ops

- `docker compose up` works from clean checkout, every main commit (CI smoke).
- `tide doctor`: postgres/redis/nats health + schema version + pending.
- Postgres down → memory-buffered ingestion continues (bounded); Redis down →
  rebuild from durable telemetry/events; NATS down → memory bus, no silent loss.
- Migrations: embedded in the binary, applied at boot (`ApplyPending`).
- Observability: OpenTelemetry from day one; every pipeline stage emits spans.
- Chaos: `tests/chaos_test.go` (always) + `TIDE_CHAOS_LIVE=1` against compose
  + scheduled `.github/workflows/chaos.yml`.
