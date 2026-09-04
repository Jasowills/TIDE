# TIDE — Telematics Infrastructure & Data Engine

Open-source infrastructure that turns raw vehicle telemetry into **state,
events, and actions**. Every fleet-tech startup re-solves the same problem —
normalizing heterogeneous telemetry (GPS trackers, OBD, CAN, OEM APIs,
Traccar, flespi, MQTT/HTTP feeds) into something usable. TIDE is the shared
layer that should exist instead.

**`Telemetry → Meaning.`** Raw signals become canonical telemetry, become
state, become events, become actions. The differentiator vs. Traccar/flespi is
not protocol handling (TIDE consumes them) — it's **state, rules, replay, and
simulation**.

> V1 targets developers and integrators building *on top of* telemetry. It is
> not a Samsara replacement — there is no fleet-ops UI beyond a
> debugging/demo console. Out of scope for V1: dispatch, payroll, driver HR,
> ELD, route optimization, navigation, fuel cards, insurance, remote engine
> control, predictive ML, AI assistant. Do not build Samsara.

## 60-second demo (zero cloud accounts, zero API keys)

```bash
docker compose up
tide doctor                                    # all services ok, schema v4
tide simulate --vehicles 100 --scenario speeding
# → vehicle.speeding.started → incident.created → webhook delivery,
#   observable at http://localhost:5173 (console) or:
curl 'http://localhost:8080/v1/events?tenant=default&type=incident.created'
tide replay --scenario speeding --vehicles 5 \
  --rule rules/speeding-alert-v1.yaml --compare rules/speeding-alert-v2.yaml
```

## How it works

```
FleetSim / MQTT / HTTP / Traccar / flespi
        │  adapters normalize to canonical telemetry (units fixed, raw kept)
        ▼
┌───────────── Pipeline (one path — replay reuses it, never a copy) ─────────┐
│ validate → dedup (sequence-first) → durable log → state engine → detectors │
│ → versioned rules (+cooldown/caps) → events on NATS → HMAC webhooks        │
└────────────────────────────────────────────────────────────────────────────┘
        │                                              │
   Postgres+PostGIS                        Redis (disposable hot state)
   (durable, partition-ready)              NATS JetStream (event bus)
```

Key properties: `.started/.continued/.ended` transition events (never
raw-packets-as-events); at-least-once delivery with deterministic,
idempotent event IDs (dedupe on `event_id`); event-time correctness with
late/out-of-order handling; per-device offline detection (never a global
threshold); deterministic replay pinned to rule versions; FleetSim with fault
injection (duplicates, late, missing, out-of-order, GPS drift, blackouts).

## Layout

```
cmd/{tide,tide-api,tide-engine,tide-sim}/  binaries + CLI (doctor/simulate/replay)
internal/{pipeline,state,detectors,rules,geo,replay,webhooks,...}  core (provider-free)
adapters/{mqtt,http,traccar,flespi}/       all provider logic lives here (lint-enforced)
schemas/{telemetry,events}/                canonical contracts — the core of the project
simulator/{scenarios,generators,faults}/   FleetSim
apps/console/                              React+TS debug console (7 screens)
sdk/typescript/                            typed client
deployments/docker/  benchmarks/  tests/   ops, repro scripts, chaos suite
rules/  examples/  openapi.yaml          sample rules, consumer example, API contract
```

## Docs

Full guides lived in `docs/` (quickstart, architecture, telemetry, events,
rules, replay, FleetSim, adapters, API, ops, security, benchmarks,
contributing) — removed from the tree; recover any page from git history.
The contract sources of truth remain: `schemas/`, `rules/`, `openapi.yaml`,
`owasp-coverage.md`.

Every claimed number ships with a reproduction script
([benchmarks/run.sh](benchmarks/run.sh)) — no unverified figures, ever.

## Non-negotiables

Canonical schema quality · provider isolation (`if provider ==` outside
`/adapters/` fails CI) · transition event semantics · idempotency ·
event-time correctness · deterministic replay via production code paths ·
FleetSim real enough for no-hardware development · OpenTelemetry from day
one · `docker compose up` works on every main commit · scope discipline.

## License

Apache 2.0 — see [LICENSE](LICENSE).
