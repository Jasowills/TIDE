# Rules (rules/*.yaml)

```yaml
id: speeding-alert
version: v1
when:
  eventType: vehicle.speeding.started
  conditions: [{field: speedKmh, op: ">", value: 120}]
then:
  emit: incident.created
  webhook: https://you.example/hook
  secret: s3cr3t
cooldownSecs: 60
maxActionsPerHour: 100
```

- Versioned + immutable once published: v2 never mutates v1 (replay pins versions).
- Every trigger persists a trace (rule, version, matched inputs, conditions,
  actions) — inspectable at `GET /v1/rules/triggers` and the Replay UI's
  "why did this fire" view.
- Safety valves ship with the engine: cooldown, dedupe, max-actions/hour.
  One bad device cannot spam your webhook endpoint.
- `tide replay --rule rules/speeding-alert-v1.yaml --compare rules/speeding-alert-v2.yaml`
  diffs versions over the same window.
