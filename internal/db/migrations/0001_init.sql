-- TIDE Phase 0 baseline migration.
-- Grilling §5.3 decision: partition strategy from day one (by tenant + time).
-- V1 runs single-partition tables; these indexes + tenant_id columns are the
-- seam that later range/list partitioning attaches to without a rewrite.

CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Phase 1 owns the full domain model; Phase 0 only proves the tooling works.
CREATE TABLE IF NOT EXISTS _phase0_probe (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
