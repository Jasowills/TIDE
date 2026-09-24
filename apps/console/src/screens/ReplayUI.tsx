import { useEffect, useState } from 'react';
import { RuleTrigger, TideEvent, fetchEvents, fetchTriggers } from '../api';

// Rule replay. Answers why an event fired: each event links to the rule
// trigger behind it (GET /v1/rules/triggers), with rule ID, version, matched
// conditions, and actions taken. The version comparison shows how two rule
// versions differ for this vehicle. Reproduce with:
// tide replay --scenario speeding --vehicles 5 \
//   --rule rules/speeding-alert-v1.yaml --compare rules/speeding-alert-v2.yaml
export function ReplayUI({ tenant, initialVehicle, focusEvent, onOpenVehicle, embedded }: {
  tenant: string;
  initialVehicle?: string;
  focusEvent?: string;
  onOpenVehicle: (v: string) => void;
  embedded?: boolean;
}) {
  const [vehicle, setVehicle] = useState(initialVehicle || 'sim-000');
  const [events, setEvents] = useState<TideEvent[]>([]);
  const [triggers, setTriggers] = useState<RuleTrigger[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [selectedEvent, setSelectedEvent] = useState<string>(focusEvent ?? '');
  const [compareA, setCompareA] = useState('');
  const [compareB, setCompareB] = useState('');

  useEffect(() => { if (initialVehicle) { setVehicle(initialVehicle); } }, [initialVehicle]);
  useEffect(() => { load(); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [tenant]);

  async function load() {
    const v = vehicle.trim();
    if (!v) return;
    setEvents(await fetchEvents(tenant, { vehicle: v }));
    setTriggers((await fetchTriggers()).filter((t) => t.VehicleID === v));
    setLoaded(true);
  }

  const versions = [...new Set(triggers.map((t) => `${t.RuleID}/${t.RuleVersion}`))];
  if (!compareA && versions[0]) setCompareA(versions[0]);
  if (!compareB && versions[1]) setCompareB(versions[1]);

  const linked = selectedEvent ? events.find((e) => e.id === selectedEvent) : undefined;
  const linkedTrigger = linked
    ? triggers.find((t) => (linked.ruleId && t.RuleID === linked.ruleId && t.RuleVersion === linked.ruleVersion) || t.VehicleID === linked.vehicleId)
    : undefined;

  const conds = (key: string) => triggers.find((t) => `${t.RuleID}/${t.RuleVersion}` === key)?.ConditionsDesc ?? [];
  const aConds = conds(compareA);
  const bConds = conds(compareB);
  const onlyA = aConds.filter((c) => !bConds.includes(c));
  const onlyB = bConds.filter((c) => !aConds.includes(c));

  return (
    <div>
      {!embedded && (
      <>
      <h2 className="tide-h">Rule Replay</h2>
      <p className="tide-sub">Deterministic replay through the production pipeline for Tenant “{tenant}”. Select an event to see the rule trigger behind it.</p>

      <div className="toolbar">
        <input className="field mono" value={vehicle} onChange={(e) => setVehicle(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter') load(); }} aria-label="Vehicle ID" placeholder="Vehicle ID" />
        <button className="btn" onClick={load}>Load Triggers</button>
        {loaded && <span className="pill info">{triggers.length} Triggers · {events.length} Events</span>}
        <button className="btn ghost" onClick={() => onOpenVehicle(vehicle)}>Inspect Vehicle</button>
      </div>
      </>
      )}

      {linked && (
        <div className="banner good">
          <b>Caused By:</b> {linked.type} ← {linkedTrigger ? `${linkedTrigger.RuleID}/${linkedTrigger.RuleVersion}` : 'Unknown Trigger'}
          {' '}· Correlation {linked.correlationId} · Caused By {linked.causationId ?? '—'}
        </div>
      )}

      <div className="stack">
        <div className="card">
          <h3>Vehicle Events <span className="tag">{events.length} Events</span></h3>
          <ul className="row-list" style={{ maxHeight: 300, overflow: 'auto' }}>
            {events.map((e) => (
              <li key={e.id} className={selectedEvent === e.id ? 'selected' : undefined}>
                <button className="row-btn" onClick={() => setSelectedEvent(e.id)}>
                  <span><b>{e.type}</b> <span style={{ color: 'var(--faint)' }}>{e.timestamp}</span></span>
                </button>
              </li>
            ))}
          </ul>
          {loaded && events.length === 0 && <p className="tide-sub">No Events Found for This Vehicle. Run a Simulation First.</p>}
        </div>
        <div className="card" style={{ background: '#050507', borderColor: '#3a3a44' }}>
          <h3 style={{ color: '#f2efe6' }}>Matched Rule Trigger {linkedTrigger && <span className="pill rule">{linkedTrigger.RuleID}/{linkedTrigger.RuleVersion}</span>}</h3>
          {linkedTrigger ? (
            <ul style={{ margin: 0, paddingLeft: 18, fontSize: 13, fontFamily: 'var(--font-mono)' }}>
              {linkedTrigger.ConditionsDesc.map((c, j) => <li key={j} style={{ marginBottom: 3 }}>✓ {c}</li>)}
              <li style={{ color: '#f2efe6', fontWeight: 600 }}>→ Actions Taken: {linkedTrigger.ActionsTaken.join(', ')}</li>
            </ul>
          ) : (
            <p className="tide-sub" style={{ margin: 0, color: '#8f8a7e' }}>{loaded ? 'Select an Event to View Its Rule Trigger.' : 'Load a Vehicle to Begin.'}</p>
          )}
        </div>
        <div className="card">
          <h3>Rule Version Comparison <span className="tag">A Versus B</span></h3>
          <div className="toolbar">
            <select className="field mono" value={compareA} onChange={(e) => setCompareA(e.target.value)} aria-label="Rule Version A">
              {versions.map((v) => <option key={v}>{v}</option>)}
            </select>
            <span style={{ color: 'var(--ink-2)', fontWeight: 700 }}>Versus</span>
            <select className="field mono" value={compareB} onChange={(e) => setCompareB(e.target.value)} aria-label="Rule Version B">
              {versions.map((v) => <option key={v}>{v}</option>)}
            </select>
          </div>
          {versions.length < 2 ? (
            <p className="tide-sub" style={{ margin: 0 }}>Two Rule Versions Are Required for Comparison. Run <code>tide replay --compare</code> to Produce Them.</p>
          ) : (
            <dl className="kv">
              <dt>Only in A</dt><dd>{onlyA.length ? onlyA.join('; ') : '—'}</dd>
              <dt>Only in B</dt><dd>{onlyB.length ? onlyB.join('; ') : '—'}</dd>
            </dl>
          )}
          <pre className="cli" style={{ marginTop: 10 }}>tide replay --scenario speeding --vehicles 5 --rule rules/speeding-alert-v1.yaml --compare rules/speeding-alert-v2.yaml</pre>
        </div>
      </div>
    </div>
  );
}
