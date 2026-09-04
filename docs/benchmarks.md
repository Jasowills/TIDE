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
