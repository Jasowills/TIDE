-- Phase 1 domain tables (T010/T011). Partition seam per ADR-001 §3:
-- every telemetry/event row carries tenant_id + event timestamp, indexed, so
-- later range/list partitioning attaches without a rewrite.

CREATE TABLE IF NOT EXISTS fleets (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS vehicles (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    fleet_id TEXT REFERENCES fleets(id),
    name TEXT NOT NULL,
    expected_cadence_secs INT NOT NULL DEFAULT 300,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_vehicles_tenant ON vehicles(tenant_id);

CREATE TABLE IF NOT EXISTS devices (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    vehicle_id TEXT NOT NULL REFERENCES vehicles(id),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS provider_identities (
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    vehicle_id TEXT NOT NULL REFERENCES vehicles(id),
    provider TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, provider, provider_id)
);

CREATE TABLE IF NOT EXISTS telemetry (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    vehicle_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    ts TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lat DOUBLE PRECISION NOT NULL,
    lng DOUBLE PRECISION NOT NULL,
    speed_kmh DOUBLE PRECISION,
    payload JSONB NOT NULL,
    raw JSONB NOT NULL,
    dedup_key TEXT NOT NULL,
    UNIQUE (tenant_id, dedup_key)
);
CREATE INDEX IF NOT EXISTS idx_telemetry_tenant_ts ON telemetry(tenant_id, ts);
CREATE INDEX IF NOT EXISTS idx_telemetry_vehicle_ts ON telemetry(vehicle_id, ts);

CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    vehicle_id TEXT NOT NULL,
    ts TIMESTAMPTZ NOT NULL,
    rule_id TEXT,
    rule_version TEXT,
    correlation_id TEXT NOT NULL,
    causation_id TEXT,
    payload JSONB,
    schema_version INT NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_events_tenant_ts ON events(tenant_id, ts);
CREATE INDEX IF NOT EXISTS idx_events_vehicle_ts ON events(vehicle_id, ts);
