import { useEffect, useRef, useState } from 'react';
import type { TideEvent } from '../api';

// Live event stream. The event log itself lives in the parent — the tape,
// the map, and this log freeze as one — while rows, filter, incidents toggle,
// and selection live here. Selecting a row opens the full trace below it:
// the rule behind the event, the correlation chain, and the raw payload.
export function EventRail({ events, paused, buffered, onPause, onResume, onOpenVehicle, onOpenReplay }: {
  events: TideEvent[];
  paused: boolean;
  buffered: number;
  onPause: () => void;
  onResume: () => void;
  onOpenVehicle: (v: string) => void;
  onOpenReplay: (v: string, e?: string) => void;
}) {
  const [filter, setFilter] = useState('');
  const [incidentsOnly, setIncidentsOnly] = useState(false);
  const [selected, setSelected] = useState<TideEvent | null>(null);
  const filterRef = useRef<HTMLInputElement>(null);

  // The "/" key focuses the filter from anywhere outside form fields.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName;
      if (e.key === '/' && tag !== 'INPUT' && tag !== 'SELECT' && tag !== 'TEXTAREA') {
        e.preventDefault();
        filterRef.current?.focus();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  const shown = events.filter(
    (e) =>
      (!incidentsOnly || e.type.startsWith('incident.')) &&
      (!filter || e.type.includes(filter) || e.vehicleId.includes(filter)),
  );

  return (
    <div>
      <div className="queue-tools">
        <input
          ref={filterRef}
          className="field mono"
          placeholder="Filter by Type or Vehicle"
          title="Press / to Focus"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          aria-label="Filter Events"
        />
        <button className="btn ghost sm" onClick={() => setIncidentsOnly((v) => !v)} aria-pressed={incidentsOnly} style={incidentsOnly ? { borderColor: '#fff', color: '#fff' } : undefined}>
          Incidents Only
        </button>
        {paused ? (
          <button className="btn sm" onClick={onResume}>Resume ({buffered} Buffered)</button>
        ) : (
          <button className="btn ghost sm" onClick={onPause}>Pause</button>
        )}
      </div>
      <ul className="row-list rail-list">
        {shown.slice(0, 100).map((e) => (
          <li key={e.id} className={[selected?.id === e.id ? 'selected' : '', e.type.startsWith('incident.') ? 'is-incident' : ''].join(' ').trim() || undefined}>
            <button className="row-btn" onClick={() => setSelected(e)} title={e.id} aria-label={`${e.type}, ${e.vehicleId}`}>
              <span className="t">{timeOf(e.timestamp)}</span>
              <span className="sev"><i className={sevKind(e.type)} /></span>
              <span className="row-type rail-type">{e.type} · {e.vehicleId}</span>
              <span className="rail-quick">
                <span
                  role="button" tabIndex={0}
                  onClick={(ev) => { ev.stopPropagation(); onOpenVehicle(e.vehicleId); }}
                  onKeyDown={(ev) => { if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); ev.stopPropagation(); onOpenVehicle(e.vehicleId); } }}
                  aria-label={`Inspect ${e.vehicleId}`}
                >Inspect</span>
                <span
                  role="button" tabIndex={0}
                  onClick={(ev) => { ev.stopPropagation(); onOpenReplay(e.vehicleId, e.id); }}
                  onKeyDown={(ev) => { if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); ev.stopPropagation(); onOpenReplay(e.vehicleId, e.id); } }}
                  aria-label={`Rule trace for ${e.type}`}
                >Rule Trace</span>
              </span>
            </button>
          </li>
        ))}
      </ul>
      {shown.length === 0 && <p className="tide-sub" style={{ margin: '8px 2px' }}>No Events Found. Run a Simulation to Generate Traffic.</p>}
      {selected && (
        <div className="rail-inspector">
          <div className="rail-inspector-head">
            <b>{selected.type}</b>
            <button className="btn ghost sm" onClick={() => setSelected(null)} aria-label="Close Inspector">Close</button>
          </div>
          <div className="chain" aria-label="Causation Chain">
            <div className="chain-node"><div className="k">Event</div><div className="v">{selected.type}</div></div>
            <div className="chain-arrow">←</div>
            <div className="chain-node cause"><div className="k">Rule</div><div className="v">{selected.ruleId ? `${selected.ruleId}/${selected.ruleVersion}` : 'No Rule (Pipeline Event)'}</div></div>
          </div>
          <dl className="kv">
            <dt>Vehicle</dt><dd>{selected.vehicleId}</dd>
            <dt>ruleId</dt><dd>{selected.ruleId ?? '—'}</dd>
            <dt>ruleVersion</dt><dd>{selected.ruleVersion ?? '—'}</dd>
            <dt>correlationId</dt><dd>{selected.correlationId}</dd>
            <dt>causationId</dt><dd>{selected.causationId ?? '—'}</dd>
          </dl>
          <div className="toolbar" style={{ margin: '10px 0 0' }}>
            <button className="btn ghost sm" onClick={() => onOpenVehicle(selected.vehicleId)}>Inspect Vehicle</button>
            <button className="btn dark sm" onClick={() => onOpenReplay(selected.vehicleId, selected.id)}>View Rule Trace</button>
          </div>
          <details style={{ marginTop: 8 }}>
            <summary className="mono-dim" style={{ cursor: 'pointer' }}>Raw Event Payload</summary>
            <pre className="dump" style={{ marginTop: 8 }}>{JSON.stringify(selected, null, 2)}</pre>
          </details>
        </div>
      )}
    </div>
  );
}

// Severity is encoded by shape, not hue: diamond for incidents, filled
// circle for transition starts, hollow circle for ends, square for the rest.
function sevKind(type: string): string {
  if (type.startsWith('incident.')) return 'k-incident';
  if (type.endsWith('.started') || type.endsWith('.entered')) return 'k-start';
  if (type.endsWith('.ended') || type.endsWith('.exited')) return 'k-end';
  return 'k-info';
}

function timeOf(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleTimeString();
}
