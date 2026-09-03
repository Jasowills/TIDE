import { useState } from 'react';
import { RuleTrigger, TideEvent, fetchEvents, fetchTriggers } from '../api';

// T105 Replay UI — pick vehicle + rule version → inspect/compare. Must answer
// "why did this event fire": triggers show matched conditions inline.
export function ReplayUI({ tenant }: { tenant: string }) {
  const [vehicle, setVehicle] = useState('sim-000');
  const [events, setEvents] = useState<TideEvent[]>([]);
  const [triggers, setTriggers] = useState<RuleTrigger[]>([]);

  async function load() {
    setEvents(await fetchEvents(tenant, { vehicle }));
    setTriggers((await fetchTriggers()).filter((t) => t.VehicleID === vehicle));
  }

  return (
    <div>
      <h2>Replay inspector</h2>
      <p>Replays run through the production pipeline (<code>tide replay</code>); this UI inspects why events fired.</p>
      <input value={vehicle} onChange={(e) => setVehicle(e.target.value)} />
      <button onClick={load}>Inspect</button>
      <h3>Why did this fire ({triggers.length} triggers)</h3>
      <ul>
        {triggers.map((t, i) => (
          <li key={i}>
            {t.RuleID}/{t.RuleVersion} at {t.At}
            <ul>
              {t.ConditionsDesc.map((c, j) => <li key={j}>{c}</li>)}
              <li>actions: {t.ActionsTaken.join(', ')}</li>
            </ul>
          </li>
        ))}
      </ul>
      <h3>Events ({events.length})</h3>
      <ul>
        {events.map((e) => <li key={e.id}>{e.timestamp} {e.type} corr={e.correlationId}</li>)}
      </ul>
    </div>
  );
}
