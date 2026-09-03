-- Phase 6 rules + webhook deliveries (T060/T063). Rules are immutable once
-- published: updates INSERT a new version row, never UPDATE.
CREATE TABLE IF NOT EXISTS rules (
    id TEXT NOT NULL,
    version TEXT NOT NULL,
    tenant_id TEXT,
    spec JSONB NOT NULL,
    published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, version)
);

CREATE TABLE IF NOT EXISTS rule_triggers (
    id BIGSERIAL PRIMARY KEY,
    rule_id TEXT NOT NULL,
    rule_version TEXT NOT NULL,
    vehicle_id TEXT NOT NULL,
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    matched_inputs JSONB,
    actions_taken JSONB
);
CREATE INDEX IF NOT EXISTS idx_triggers_rule ON rule_triggers(rule_id, rule_version);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT NOT NULL,
    url TEXT NOT NULL,
    status INT,
    error TEXT,
    dead_letter BOOL NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_webhook_event ON webhook_deliveries(event_id);
