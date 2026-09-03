# Replay (internal/replay)

Replay runs the SAME `Pipeline.Process` as production — there is no parallel
implementation (a separate replay engine is an automatic design rejection).

Determinism pins: telemetry + config + rule version + schema version +
algorithm version, all caller inputs, never live data. Event ids are
deterministic, so two runs of one fixture are byte-identical
(T071 harness asserts this in CI).

```bash
tide replay --scenario speeding --vehicles 5 --rule rules/speeding-alert-v1.yaml
tide replay --input recording.json --rule rules/speeding-alert-v1.yaml --compare rules/speeding-alert-v2.yaml
```
