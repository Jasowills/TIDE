# Contributing

- Spec Package v1 is source of truth until ADRs supersede it.
- Definition of Done: unit+integration tests pass; no provider naming outside
  `/adapters/`; OTel spans for new stages; `docker compose up` works;
  docs updated; schema changes additive (or ADR + version bump).
- Canonical schema changes: ADR + second reviewer, always.
- Tickets: Given/When/Then acceptance, one test layer named.
- Out of scope for V1 (rejected, not descoped): dispatch, payroll, driver HR,
  ELD, route optimization, navigation, fuel cards, insurance, remote engine
  control, predictive ML, AI assistant. Do not build Samsara.
