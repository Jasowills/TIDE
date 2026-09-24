import { useEffect, useState } from 'react';
import { TideEvent, fetchEvents } from '../api';

// Event timeline: one vehicle's history on a time axis instead of a list.
// Markers reuse the stream's shape language (diamond for incidents). The axis
// spans the vehicle's first to last recorded event; selecting a marker opens
// the full trace beneath it. Everything shown comes from GET /v1/events.
function kind(e: TideEvent): 'inc' | 'start' | 'end' | 'info' {
  if (e.type.startsWith('incident.')) return 'inc';
  if (e.type.endsWith('.started') || e.type.endsWith('.entered')) return 'start';
  if (e.type.endsWith('.ended') || e.type.endsWith('.exited')) return 'end';
  return 'info';
}

function spanLabel(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 60) return `${s}s Span`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s Span`;
  return `${Math.floor(m / 60)}h ${m % 60}m Span`;
}

export function VehicleTimeline({ tenant, vehicle }: { tenant: string; vehicle: string }) {
  const [events, setEvents] = useState<TideEvent[]>([]);
  const [failed, setFailed] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const id = vehicle.trim();

  useEffect(() => {
    setSelectedId(null);
    setFailed(false);
    if (!id) {
      setEvents([]);
      return;
    }
    fetchEvents(tenant, { vehicle: id })
      .then((e) => setEvents(e))
      .catch(() => {
        setEvents([]);
        setFailed(true);
      });
  }, [tenant, id]);

  const selected = selectedId ? events.find((e) => e.id === selectedId) : undefined;

  if (!id) {
    return (
      <div className="card">
        <h3>Event Timeline <span className="tag">Time Axis</span></h3>
        <p className="tide-sub" style={{ margin: 0 }}>Enter a Vehicle ID Above to Plot Its History.</p>
      </div>
    );
  }

  if (events.length === 0) {
    return (
      <div className="card">
        <h3>Event Timeline <span className="tag">{id}</span></h3>
        <p className="tide-sub" style={{ margin: 0 }}>
          {failed
            ? `Could Not Load History for Vehicle “${id}”. Verify the API Is Running.`
            : `No Events Found for Vehicle “${id}”. Run a Simulation to Generate Traffic.`}
        </p>
      </div>
    );
  }

  const times = events
    .map((e) => new Date(e.timestamp).getTime())
    .filter((t) => !Number.isNaN(t));
  const t0 = Math.min(...times);
  const t1 = Math.max(...times, t0 + 1000);
  // Axis insets match the .tl-axis gutters so markers sit on the line.
  const X = (t: number) => 2.5 + ((t - t0) / (t1 - t0)) * 95;

  return (
    <div className="card">
      <h3>Event Timeline <span className="tag">{events.length} Events · {spanLabel(t1 - t0)}</span></h3>
      <div className="tl" role="group" aria-label={`Event timeline for ${id}`}>
        <div className="tl-axis" aria-hidden />
        {events.map((e) => {
          const t = new Date(e.timestamp).getTime();
          if (Number.isNaN(t)) return null;
          const sel = e.id === selectedId;
          return (
            <button
              key={e.id}
              className="tl-mark"
              style={{ left: `${X(t)}%` }}
              onClick={() => setSelectedId(sel ? null : e.id)}
              title={`${e.type} · ${e.timestamp}`}
              aria-label={`${e.type} at ${e.timestamp}`}
              aria-pressed={sel}
            >
              <span className="tl-dot" data-k={kind(e)} data-sel={sel ? '1' : undefined} />
            </button>
          );
        })}
        <span className="tl-edge" aria-hidden />
      </div>
      <div className="tl-scale">
        <span>{new Date(t0).toLocaleString()}</span>
        <span>{new Date(t1).toLocaleString()}</span>
      </div>
      {selected && (
        <div style={{ marginTop: 12 }}>
          <div className="chain" aria-label="Causation Chain">
            <div className="chain-node">
              <div className="k">Event</div>
              <div className="v">{selected.type}</div>
            </div>
            <div className="chain-arrow">←</div>
            <div className="chain-node cause">
              <div className="k">Rule</div>
              <div className="v">{selected.ruleId ? `${selected.ruleId}/${selected.ruleVersion}` : 'No Rule (Pipeline Event)'}</div>
            </div>
          </div>
          <dl className="kv">
            <dt>Time</dt>
            <dd>{selected.timestamp}</dd>
            <dt>Correlation</dt>
            <dd>{selected.correlationId}</dd>
            <dt>Caused By</dt>
            <dd>{selected.causationId ?? '—'}</dd>
          </dl>
          <details style={{ marginTop: 8 }}>
            <summary className="mono-dim" style={{ cursor: 'pointer' }}>Raw Event Payload</summary>
            <pre className="dump" style={{ marginTop: 8 }}>{JSON.stringify(selected, null, 2)}</pre>
          </details>
        </div>
      )}
    </div>
  );
}
