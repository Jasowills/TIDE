# API (openapi.yaml is the contract)

- `POST /v1/telemetry`, `POST /v1/telemetry:batch` → 202 `{accepted, events}`
- `GET /v1/events?tenant=&vehicle=&type=` (tenant required, always scoped)
- `GET /v1/vehicles/{id}/state`
- `GET|POST /v1/geofences`
- `GET /v1/rules/triggers` (evaluation trace)
- `GET /v1/connections` (adapter health)
- `WS /v1/stream` (live events)
- `GET /healthz`, `/readyz`

V1 auth: API keys / service tokens only (OAuth2/SSO deferred).
See `sdk/typescript` for the generated-friendly client.
