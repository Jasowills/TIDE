import { useEffect, useState } from 'react';
import { Connection, fetchConnections } from '../api';

// Adapter health (GET /v1/connections). Degraded adapters sort first so the
// failure is visible without scanning. Refreshes every ten seconds with the
// last refresh time shown, so staleness is honest.
export function Connections() {
  const [conns, setConns] = useState<Connection[]>([]);
  const [updatedAt, setUpdatedAt] = useState<number | null>(null);
  const [now, setNow] = useState(Date.now());

  useEffect(() => {
    const load = () => {
      fetchConnections().then((c) => { setConns(c); setUpdatedAt(Date.now()); }).catch(() => {});
    };
    load();
    const t = setInterval(load, 10_000);
    const clock = setInterval(() => setNow(Date.now()), 1000);
    return () => { clearInterval(t); clearInterval(clock); };
  }, []);

  const healthy = conns.filter((c) => c.state === 'HEALTHY').length;
  const ago = updatedAt ? Math.max(0, Math.round((now - updatedAt) / 1000)) : null;
  const ordered = [...conns].sort((a, b) => {
    const rank = (s: string) => (s === 'HEALTHY' ? 2 : s === 'CONFIGURED' ? 1 : 0);
    return rank(a.state) - rank(b.state);
  });

  return (
    <div>
      <h2 className="tide-h">Adapter Connections</h2>
      <p className="tide-sub">Health of every telemetry adapter{ago !== null ? `, refreshed ${ago} seconds ago` : ''}. Degraded adapters are listed first.</p>

      {conns.length === 0 ? (
        <div className="card">
          <p className="tide-sub" style={{ margin: 0 }}>No Adapter Data. Start the Stack With docker compose up.</p>
        </div>
      ) : (
      <>
      <div className="card" style={{ marginBottom: 14 }}>
        <h3>Adapter Health <span className="tag">{healthy} of {conns.length} Healthy</span></h3>
        <div className="progress" role="progressbar" aria-valuenow={conns.length ? Math.round((healthy / conns.length) * 100) : 0} aria-valuemin={0} aria-valuemax={100} aria-label="Healthy Adapter Share">
          <div style={{ width: `${conns.length ? (healthy / conns.length) * 100 : 0}%` }} />
        </div>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 12 }}>
          {ordered.map((c) => {
            const bad = c.state !== 'HEALTHY' && c.state !== 'CONFIGURED';
            return (
              <span key={c.name} className={`pill ${c.state === 'HEALTHY' ? 'ok' : bad ? 'bad' : 'mute'}`}>
                {c.name}: {c.state === 'CONFIGURED' ? 'Not Connected' : c.state}
              </span>
            );
          })}
        </div>
      </div>

      <div className="card table-scroll" style={{ padding: 6 }}>
        <table className="tide">
          <thead>
            <tr><th>Adapter</th><th>State</th><th>Messages/Sec</th><th>Devices</th><th>Last Seen</th></tr>
          </thead>
          <tbody>
            {ordered.map((c) => {
              const bad = c.state !== 'HEALTHY' && c.state !== 'CONFIGURED';
              const ok = c.state === 'HEALTHY';
              return (
                <tr key={c.name} className={bad ? 'row-bad' : undefined}>
                  <td style={{ color: 'var(--ink)', fontWeight: 600 }}>{c.name}</td>
                  <td>
                    <span className={`pill ${ok ? 'ok' : bad ? 'bad' : 'mute'}`}>{c.state === 'CONFIGURED' ? 'Not Connected' : c.state}</span>
                  </td>
                  <td>{c.msgPerSec ?? '—'}</td>
                  <td>{c.deviceCount ?? '—'}</td>
                  <td style={{ color: 'var(--ink-2)' }}>{c.lastSeen ?? '—'}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      </>
      )}
    </div>
  );
}
