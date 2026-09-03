# FleetSim (simulator/)

Real enough to develop and demo without hardware. Scenarios are YAML
(`simulator/scenarios/*.yaml`): mix of normal/speeding/idling/offline
profiles, deterministic in seed. Fault injection from V1 (never a stretch goal):

```bash
tide simulate --vehicles 100 --scenario speeding \
  --duplicate-events 0.02 --late-events 0.02 --out-of-order 0.02 \
  --missing-events 0.01 --gps-drift 50 --offline-vehicles 0.1
```

Output is canonical telemetry, indistinguishable at the ingestion boundary
from a real adapter — which is what makes replay/rule-testing credible.
