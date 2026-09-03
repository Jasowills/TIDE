import { useState } from 'react';
import { TideEvent, VehicleState, fetchEvents, fetchState } from '../api';

// T102 Vehicle detail — state, identity, event + raw telemetry history with
// correlation/causation IDs surfaced inline (§4.3).
export function VehicleDetail({ tenant }: { tenant: string }) {
  const [id, setId] = useState('sim-000');
  const [state, setState] = useState<VehicleState | null>(null);
  const [events, setEvents] = useState<TideEvent[]>([]);
  const [err, setErr] = useState('');

  async function load() {
    setErr('');
    try {
      setState(await fetchState(id));
      setEvents(await fetchEvents(tenant, { vehicle: id }));
    } catch (e) {
      setErr(String(e));
    }
  }

  return (
    <div>
      <h2>Vehicle detail</h2>
      <input value={id} onChange={(e) => setId(e.target.value)} />
      <button onClick={load}>Load</button>
      {err && <p style={{ color: 'red' }}>{err}</p>}
      {state && (
        <dl>
          <dt>motion</dt><dd>{state.motion}</dd>
          <dt>presence</dt><dd>{state.presence}</dd>
          <dt>speed</dt><dd>{state.speedKmh} km/h</dd>
          <dt>position</dt><dd>{state.lat}, {state.lng}</dd>
          <dt>last seen</dt><dd>{state.lastSeen}</dd>
          <dt>trip</dt><dd>{state.tripId ?? '—'}</dd>
        </dl>
      )}
      <h3>Event history</h3>
      <ul>
        {events.map((e) => (
          <li key={e.id}>
            {e.timestamp} {e.type} corr={e.correlationId} cause={e.causationId ?? '—'}
            {e.ruleId && ` rule=${e.ruleId}/${e.ruleVersion}`}
          </li>
        ))}
      </ul>
    </div>
  );
}
