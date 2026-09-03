# Adapters (adapters/)

| Adapter | Path | Capabilities |
|---|---|---|
| Generic MQTT (YAML field-mapping, zero Go code) | `adapters/mqtt` | live |
| HTTP ingestion (`POST /v1/telemetry`, `:batch`) | `internal/ingest` | live |
| Traccar REST (pinned: Traccar 6.x API) | `adapters/traccar` | live, history, devices |
| flespi MQTT live + REST discovery | `adapters/flespi` | live, history, devices, webhooks |

Provider logic lives ONLY in `/adapters/<provider>` (lint-enforced).
Traccar/flespi are contract-tested against recorded fixtures in CI, never
live systems (Grilling §5.1). Unit conversion is adapter work (knots→km/h),
never core. Lifecycle: CONFIGURED → CONNECTING → HEALTHY → DEGRADED →
RECONNECTING → FAILED, backoff+jitter mandatory.
