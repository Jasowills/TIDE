# Benchmarks

Owner: Jason (maintainer). Method: `benchmarks/run.sh`. Latest run: see
`benchmarks/last.txt` (hardware + commit recorded with every run).

Targets (engineering targets, not guarantees): HTTP ingestion p95 < 100ms,
state update p95 < 100ms, rule evaluation p95 < 100ms, event propagation
p95 < 250ms. No number below — or anywhere in docs — ships without a script.
