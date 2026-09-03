# Architecture — modular monolith

`Telemetry → Meaning`: adapters normalize to canonical telemetry → pipeline
(validate → dedup → durable log → state → detectors → rules) → events on NATS
→ webhooks + console.

Binaries: `tide-api` (REST+WS, ingestion, queries), `tide-engine` (offline
sweeper, MQTT subscriber), `tide` (doctor/simulate/replay CLI), `tide-sim`.

Storage: Postgres+PostGIS (durable, partition-ready via tenant_id+ts),
Redis (disposable hot state — rebuilds from telemetry, never source of
truth), NATS JetStream local (Kafka-compatible later).

Hard rules: no `if provider ==` outside `/adapters/` (lint); replay uses the
production `Pipeline.Process` only; rules immutable once published;
at-least-once + idempotent consumers (dedupe on `event_id`); event-time
semantics with watermark = LastSeen.
