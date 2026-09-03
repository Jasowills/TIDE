# ADR-001 — Grilling §5 pre-Phase-0 decisions (one-line each, binding)

Date: 2026-09-03. Required before Phase 0 tickets open (Spec §5 Action).

1. **Traccar/flespi coupling (§5.1):** adapters version-pin against a known upstream API version and are contract-tested against recorded fixtures, never live systems, in CI. (Enforced in T090/T091.)
2. **Modular-monolith boundaries (§5.2):** `scripts/check-provider-isolation.sh` runs in CI from commit #1; any `if provider ==` outside `/adapters/` fails the build.
3. **Postgres-for-telemetry ceiling (§5.3):** tables carry `tenant_id` + time indexes from day one as the partition seam (see `migrations/0001_init.sql`); range/list partitioning attaches later without rewrite. V1 scale target documented in benchmarks; no sharding in V1.
4. **Replay vs external data (§5.4, deferred):** tracked risk — any rule input outside the telemetry stream must record version/provider at run time. Decided before post-V1 Context Engine, not now.
5. **At-least-once webhook promise (§5.5, deferred):** tracked risk — public docs teach consumers to dedupe on `event_id` from V1 (T063/T113).
6. **FleetSim seriousness (§5.6):** fault injection (duplicate/late/missing/out-of-order/gps-drift/offline) is Phase 8 core scope (T082), staffed like the state engine, never a stretch goal.
7. **Canonical schema review (§5.7):** every schema change requires an ADR + a second reviewer, no matter how small; breaking changes need a version bump (DoD).
8. **Benchmark ownership (§5.8, deferred):** tracked risk — T111 names an owner; no number ships in docs without a checked-in repro script.
9. **License boundary (§5.9, deferred):** tracked risk — Apache 2.0 core; decide core-forever vs future-paid boundary before external PRs touch RBAC/SSO areas.
10. **Solo-builder slicing (§5.10):** after every phase, the §7 vertical slice must stay green before breadth work (Phase 9 adapters, rest of Phase 10 console) begins.
