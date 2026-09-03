-- Phase 5 geofences (T050). PostGIS geometry is the durable query path;
-- Hot path pre-filters in Go (see internal/geo CandidateFilter).
CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE IF NOT EXISTS geofences (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    geom GEOMETRY(Polygon, 4326) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_geofences_tenant ON geofences(tenant_id);
CREATE INDEX IF NOT EXISTS idx_geofences_geom ON geofences USING GIST (geom);
