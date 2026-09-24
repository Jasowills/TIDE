import { useState } from 'react';
import type { TideEvent } from '../../api';
import { getRoster, timeAgo } from './model';

// Fleet roster: every vehicle in the current log, incidents first, then most
// recent. The search box filters by ID or last event type. Selecting a row
// loads the vehicle into the dossier.
export function Roster({ events, selected, onSelect }: {
  events: TideEvent[];
  selected: string | null;
  onSelect: (v: string) => void;
}) {
  const [q, setQ] = useState('');
  const rows = getRoster(events);
  const needle = q.trim().toLowerCase();
  const shown = needle
    ? rows.filter((r) => r.id.toLowerCase().includes(needle) || r.lastType.toLowerCase().includes(needle))
    : rows;

  return (
    <section className="panel" aria-label="Fleet Roster">
      <div className="panel-h">
        <h3>Fleet Roster</h3>
        <span className="mono-dim">{rows.length} Vehicles</span>
      </div>
      <div className="roster-search">
        <input
          className="field mono"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Search Vehicles"
          aria-label="Search Fleet Roster"
        />
      </div>
      {rows.length === 0 ? (
        <p className="panel-empty">No Vehicles Reporting. Run a Simulation to Generate Traffic.</p>
      ) : shown.length === 0 ? (
        <p className="panel-empty">No Vehicles Match This Filter.</p>
      ) : (
        <ul className="queue-list roster-list">
          {shown.map((r) => (
            <li key={r.id} className={r.id === selected ? 'selected' : undefined}>
              <button onClick={() => onSelect(r.id)} aria-label={`${r.id}, last event ${r.lastType}`}>
                <span className={`rk rk-${r.severity === 'inc' ? 'inc' : r.severity === 'start' ? 'start' : 'rest'}`} aria-hidden />
                <span className="q-main">
                  <span className="q-type">{r.id}</span>
                  <span className="q-veh">
                    {r.lastType} · {timeAgo(r.lastAt)}
                  </span>
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
