import { useEffect, useState } from 'react';
import { Connection, fetchConnections } from '../api';

// T104 Connections — per-adapter health, device count, msg/sec, connection
// state. Degraded states are visually distinct (§4.3).
export function Connections() {
  const [conns, setConns] = useState<Connection[]>([]);

  useEffect(() => {
    const load = () => fetchConnections().then(setConns).catch(() => {});
    load();
    const t = setInterval(load, 10_000);
    return () => clearInterval(t);
  }, []);

  return (
    <div>
      <h2>Connections</h2>
      <table>
        <thead>
          <tr><th>adapter</th><th>state</th><th>msg/sec</th><th>devices</th><th>last seen</th></tr>
        </thead>
        <tbody>
          {conns.map((c) => (
            <tr key={c.name} style={c.state !== 'HEALTHY' ? { background: '#4a1010' } : undefined}>
              <td>{c.name}</td>
              <td>{c.state}</td>
              <td>{c.msgPerSec ?? '—'}</td>
              <td>{c.deviceCount ?? '—'}</td>
              <td>{c.lastSeen ?? '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
