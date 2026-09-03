import { useEffect, useState } from 'react';
import { TideEvent, fetchEvents, streamEvents } from '../api';

// T103 Event explorer — THE primary debugging tool: every event inspectable
// with full payload + trace IDs, live-updating. Layout priority accordingly.
export function EventExplorer({ tenant }: { tenant: string }) {
  const [events, setEvents] = useState<TideEvent[]>([]);
  const [filter, setFilter] = useState('');
  const [selected, setSelected] = useState<TideEvent | null>(null);

  useEffect(() => {
    fetchEvents(tenant).then((e) => setEvents(e.reverse())).catch(() => {});
    return streamEvents((e) => {
      if (e.tenantId === tenant) setEvents((prev) => [e, ...prev].slice(0, 500));
    });
  }, [tenant]);

  const shown = events.filter((e) => !filter || e.type.includes(filter) || e.vehicleId.includes(filter));

  return (
    <div style={{ display: 'flex', gap: 24 }}>
      <div style={{ flex: 1 }}>
        <h2>Event explorer ({shown.length})</h2>
        <input placeholder="filter type/vehicle" value={filter} onChange={(e) => setFilter(e.target.value)} />
        <ul>
          {shown.slice(0, 100).map((e) => (
            <li key={e.id}>
              <button onClick={() => setSelected(e)}>
                {e.type} · {e.vehicleId}
              </button>
            </li>
          ))}
        </ul>
      </div>
      <div style={{ flex: 1 }}>
        <h3>Inspector</h3>
        {selected ? (
          <pre>{JSON.stringify(selected, null, 2)}</pre>
        ) : (
          <p>Select an event. Rule id+version, correlationId, causationId always visible.</p>
        )}
      </div>
    </div>
  );
}
