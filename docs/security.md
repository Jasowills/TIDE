# Security (T112 audit notes)

- Tenant isolation at the query layer: every event/state query requires and
  filters by tenant; unscoped `/v1/events` is rejected (tested:
  `TestTenantIsolation`). Never trust `tenantId` from a request body.
- Secrets: webhook secrets via env, never logged, never returned by APIs.
- Webhooks: HMAC-SHA256 over `timestamp.eventId.body`, 5-min timestamp
  tolerance (replay-resistant). Consumers: verify + dedupe on `event_id`
  (see examples/webhook-consumer.py).
- V1 RBAC: platform_admin, tenant_admin, fleet_manager, operator, developer,
  viewer (resource:action) — enforced at API boundary as endpoints grow.
- Adapters are trusted in-process code in V1 (no third-party plugin sandbox).
