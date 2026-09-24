import { useEffect, useState } from 'react';
import { TideEvent, VehicleState, fetchEvents, fetchState } from '../api';

const FALLBACKS = ['sim-000', 'sim-001', 'sim-002'];

// Canonical vehicle state (GET /v1/vehicles/:id/state) plus the transition
// history for that vehicle, newest first with Correlation and Causation IDs.
export function VehicleDetail({ tenant, initialVehicle, onOpenReplay, embedded }: {
  tenant: string;
  initialVehicle?: string;
  onOpenReplay: (v: string) => void;
  embedded?: boolean;
}) {
  const [id, setId] = useState(initialVehicle || 'sim-000');
  const [state, setState] = useState<VehicleState | null>(null);
  const [events, setEvents] = useState<TideEvent[]>([]);
  const [err, setErr] = useState('');
  const [loading, setLoading] = useState(false);
  const [recent, setRecent] = useState<string[]>(() => {
    try { return JSON.parse(localStorage.getItem('tide-recent') ?? '[]'); } catch { return []; }
  });

  useEffect(() => { if (initialVehicle) setId(initialVehicle); }, [initialVehicle]);

  async function load(target = id) {
    const v = target.trim();
    if (!v) { setErr('Enter a Vehicle ID. Recent and sample IDs are listed below.'); return; }
    setErr('');
    setLoading(true);
    try {
      setState(await fetchState(v));
      setEvents(await fetchEvents(tenant, { vehicle: v }));
      setRecent((prev) => {
        const next = [v, ...prev.filter((x) => x !== v)].slice(0, 6);
        try { localStorage.setItem('tide-recent', JSON.stringify(next)); } catch { /* ignore */ }
        return next;
      });
    } catch (e) {
      setState(null);
      setEvents([]);
      setErr(`Could Not Load Vehicle “${v}” in Tenant “${tenant}”. Verify the ID and Tenant, Then Retry. (${String(e)})`);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { load(); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [tenant]);

  return (
    <div>
      {!embedded && (
      <>
      <h2 className="tide-h">Vehicle State</h2>
      <p className="tide-sub">Canonical state and transition history for one vehicle in Tenant “{tenant}”.</p>

      <div className="toolbar">
        <input className="field mono" value={id} onChange={(e) => setId(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter') load(); }} aria-label="Vehicle ID" placeholder="Vehicle ID" style={{ width: 220 }} list="tide-vehicles" />
        <datalist id="tide-vehicles">
          {[...recent, ...FALLBACKS].map((v) => <option key={v} value={v} />)}
        </datalist>
        <button className="btn" onClick={() => load()} disabled={loading}>{loading ? 'Loading' : 'Load Vehicle'}</button>
        {state && <span className={`pill ${state.motion === 'moving' ? 'info' : 'mute'}`}>{state.motion}</span>}
        {state && <span className="pill ok">{state.presence}</span>}
        <button className="btn ghost" onClick={() => onOpenReplay(id)}>View Rule Trace</button>
      </div>

      {recent.length > 0 && (
        <div className="vehicle-links" aria-label="Recently Viewed Vehicles">
          {recent.map((v) => <button key={v} onClick={() => { setId(v); load(v); }}>{v}</button>)}
        </div>
      )}
      </>
      )}

      {err && <div className="banner bad" role="alert" style={{ marginTop: 12 }}>{err}</div>}

      <div className="stack" style={{ marginTop: embedded ? 0 : 12 }}>
        <div className="card">
          <h3>Current State <span className="tag">{id}</span></h3>
          {state ? (
            <>
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 12 }}>
                <span className={`pill ${state.motion === 'moving' ? 'info' : 'mute'}`}>{state.motion}</span>
                <span className="pill ok">{state.presence}</span>
                {state.tripId && <span className="pill rule">Trip {state.tripId}</span>}
              </div>
              <dl className="kv">
                <dt>Speed</dt><dd>{state.speedKmh} km/h</dd>
                <dt>Position</dt><dd>{state.lat}, {state.lng}</dd>
                <dt>Last Seen</dt><dd>{state.lastSeen}</dd>
                <dt>Trip ID</dt><dd>{state.tripId ?? '—'}</dd>
              </dl>
            </>
          ) : (
            !err && (loading ? (
              <div style={{ display: 'grid', gap: 8 }} aria-label="Loading Vehicle State">
                <div className="shimmer" style={{ height: 14 }} />
                <div className="shimmer" style={{ height: 14, width: '70%' }} />
                <div className="shimmer" style={{ height: 14, width: '85%' }} />
              </div>
            ) : (
              <p className="tide-sub" style={{ margin: 0 }}>Enter a Vehicle ID and Select Load Vehicle.</p>
            ))
          )}
        </div>
        <div className="card">
          <h3>Transition History <span className="tag">{events.length} Events</span></h3>
          {events.length === 0 ? (
            <p className="tide-sub" style={{ margin: 0 }}>No Events Found for This Vehicle in Tenant “{tenant}”. Run a Simulation to Generate Traffic.</p>
          ) : (
            <ul className="row-list" style={{ maxHeight: 380, overflow: 'auto' }}>
              {events.map((e) => (
                <li key={e.id} className={e.type.startsWith('incident.') ? 'is-incident' : undefined}>
                  <div style={{ padding: '9px 12px', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
                    <div style={{ color: 'var(--ink)', fontWeight: 600 }}><span className="sev"><i className={sevKind(e.type)} />{e.type}</span></div>
                    <div style={{ color: 'var(--ink-2)', fontSize: 11, marginTop: 3 }}>
                      {e.timestamp} · Correlation {e.correlationId} · Caused By {e.causationId ?? '—'}
                      {e.ruleId && ` · Rule ${e.ruleId}/${e.ruleVersion}`}
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </div>
  );
}

function sevKind(type: string): string {
  if (type.startsWith('incident.')) return 'k-incident';
  if (type.endsWith('.started')) return 'k-start';
  return 'k-info';
}
