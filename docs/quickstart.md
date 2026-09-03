# TIDE quickstart — zero cloud accounts, zero API keys

```bash
docker compose up            # postgres, redis, nats, mosquitto, api, engine, console
tide doctor                  # all checks ok, schema version + pending
tide simulate --vehicles 100 --scenario speeding   # → incident.created → webhook
```

Watch it: console at http://localhost:5173 (Overview + Event explorer),
or query directly:

```bash
curl 'http://localhost:8080/v1/events?tenant=default&type=incident.created'
tide replay --scenario speeding --vehicles 5 --rule rules/speeding-alert-v1.yaml
```

See: architecture.md, telemetry.md, events.md, rules.md, replay.md,
fleetsim.md, adapters.md, api.md, sdk.md, ops.md, security.md, contributing.md.
Benchmarks: benchmarks/run.sh (no number ships without a script).
