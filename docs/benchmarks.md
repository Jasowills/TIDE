# Benchmarks

Owner: Jason (maintainer). Method: `benchmarks/run.sh`. Latest run: see
`benchmarks/last.txt` (hardware + commit recorded with every run).

Targets (engineering targets, not guarantees): HTTP ingestion p95 < 100ms,
state update p95 < 100ms, rule evaluation p95 < 100ms, event propagation
p95 < 250ms. No number below — or anywhere in docs — ships without a script.

## v0.1.0 (recorded 2026-09-04)

Hardware: Intel i7-9750H (MacBook Pro, darwin/amd64, Docker Desktop services).
Method: `go test -bench=. -benchtime=10000x ./benchmarks/` + batch-POST timing
against local compose stack (PG+NATS backed). NOT tuned, NOT production hardware.

| Benchmark | Result |
|---|---|
| Pipeline ingest, 1 vehicle (in-process) | 5,263 ns/op (~190k pts/sec) |
| Pipeline ingest, 100 vehicles (in-process) | 3,701 ns/op (~270k pts/sec) |
| HTTP `POST :batch`, 100 pts, median of 5 (local dev) | 117.8 ms/batch (~1.2 ms/pt) |

In-process numbers cover validate → dedup → log → state → detectors.
HTTP numbers include container networking, PG writes, and NATS publish on a
laptop — treat as a floor for local dev, not a claim about production.

## CI budgets (enforced, not documented wishes)

k6 thresholds (`load-tests/*.js`) and LHCI assertions
(`apps/console/lighthouserc.cjs`) fail the build on breach. Measured
2026-09-04 on local compose (i7-9750H), budgets set with headroom for shared
CI runners. Workflow: `.github/workflows/performance.yml` (per-PR baseline +
spike + Lighthouse; weekly soak).

| Budget | Measured | Gate |
|---|---|---|
| Ingest batch p95 | 68ms | `<1500ms` (`flow:ingest`) |
| Ingest batch p99 | ~200ms | `<3000ms` (`flow:ingest`) |
| Query p95 | 9ms | `<500ms` (`flow:queries`) |
| Failed request rate | 0.00% | `<1%` overall, `<1%` in recovery |
| Post-spike recovery p95 | 184ms | `<1800ms` (`phase:recovery`) — recovery is asserted, not commented |

Console Core Web Vitals baselines (lab, desktop preset, 3-run median):

| Page | LCP | CLS | TBT | Perf | Gate |
|---|---|---|---|---|---|
| `/` (Overview) | 0.38s ✅ Good | 0.005 ✅ Good | 0ms ✅ | 1.0 | LCP≤2.5s, CLS≤0.1, TBT≤200ms, perf≥0.9, interactive≤5s |

INP is field-only and is never asserted in lab (TBT is the proxy); real INP
comes from CrUX/RUM once the console has production users. FID is never
asserted anywhere (deprecated, removed from web-vitals v5+).
