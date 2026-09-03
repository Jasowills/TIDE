import { useEffect, useState } from 'react';
import { TideEvent, fetchConnections, fetchEvents, streamEvents } from '../api';

// T100 Overview — a SYSTEMS dashboard, not a fleet-ops dashboard:
// vehicle counts, events today, critical incidents, telemetry posture,
// adapter health (impossible to miss when degraded — §4.3).
export function Overview({ tenant }: { tenant: string }) {
  const [events, setEvents] = useState<TideEvent[]>([]);
  const [conns, setConns] = useState<{ name: string; state: string }[]>([]);
  const [live, setLive] = useState(0);

  useEffect(() => {
    fetchEvents(tenant).then(setEvents).catch(() => {});
    fetchConnections().then(setConns).catch(() => {});
    return streamEvents(() => setLive((n) => n + 1));
  }, [tenant]);

  const byType = new Map<string, number>();
  for (const e of events) byType.set(e.type, (byType.get(e.type) ?? 0) + 1);
  const incidents = events.filter((e) => e.type.startsWith('incident.')).length;
  const degraded = conns.filter((c) => c.state !== 'HEALTHY');

  return (
    <div>
      <h2>Overview — what is TIDE doing</h2>
      {degraded.length > 0 && (
        <div style={{ background: '#4a1010', padding: 12, marginBottom: 12 }}>
          DEGRADED: {degraded.map((d) => `${d.name} (${d.state})`).join(', ')}
        </div>
      )}
      <div style={{ display: 'flex', gap: 24 }}>
        <Stat label="events observed" value={events.length} />
        <Stat label="critical incidents" value={incidents} />
        <Stat label="live events this session" value={live} />
        <Stat label="adapters degraded" value={degraded.length} />
      </div>
      <h3>Events by type</h3>
      <ul>
        {[...byType.entries()].map(([t, n]) => (
          <li key={t}>
            {t}: {n}
          </li>
        ))}
      </ul>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <div style={{ fontSize: 28 }}>{value}</div>
      <div>{label}</div>
    </div>
  );
}
