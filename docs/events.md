# Events (schemas/events)

Transition-based names only: `.started/.continued/.ended` (motion, speeding,
trips, offline), `.entered/.exited` (geofences), `.created` (one-time
occurrences like `incident.created`). Raw packets are never events.

Every event carries: id (deterministic from type+vehicle+time+cause —
redelivery is idempotent), tenantId, vehicleId, timestamp, ruleId/ruleVersion
(when rule-triggered), correlationId, causationId, payload, schemaVersion.

Delivery: at-least-once. **Consumers must dedupe on `event_id`**
(see examples/webhook-consumer.py). NATS subject: `tide.events.<tenant>.<type>`.
