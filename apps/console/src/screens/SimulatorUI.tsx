import { useState } from 'react';

// FleetSim run configuration. Three steps: select a traffic scenario, set
// fault injection rates, start the run. The equivalent tide simulate command
// is shown for every configuration so runs are reproducible from the CLI.
// Run history persists in this browser only.
type Run = { id: string; scenario: string; vehicles: number; at: string; result: string };

const SCENARIOS = [
  { key: 'normal', ds: 'Steady Trips, No Faults' },
  { key: 'speeding', ds: 'Speeding Violations and Incidents' },
  { key: 'idling', ds: 'Extended Stops and Dwell Periods' },
  { key: 'offline', ds: 'Connectivity Gaps and Late Data' },
  { key: 'mixed', ds: 'Combined Traffic Pattern' },
];

// Waveform glyphs: each scenario drawn as the signal it generates, so the
// choice reads at a glance before a single run starts.
const WAVES: Record<string, string> = {
  normal: 'M2,20 L12,20 L16,13 L20,20 L46,20',
  speeding: 'M2,20 L9,20 L12,5 L15,20 L24,20 L27,9 L30,20 L46,20',
  idling: 'M2,20 L30,20 L33,13 L36,20 L46,20',
  offline: 'M2,20 L13,20 M19,20 L29,20 M35,20 L46,20',
  mixed: 'M2,20 L7,15 L12,22 L17,11 L22,20 L28,15 L33,22 L39,12 L46,18',
};

function WaveGlyph({ d, active }: { d: string; active: boolean }) {
  return (
    <svg
      viewBox="0 0 48 28"
      width="100%"
      height="30"
      aria-hidden="true"
      style={{
        display: 'block',
        marginBottom: 8,
        color: active ? '#ffffff' : '#76716a',
        filter: active ? 'drop-shadow(0 0 4px rgba(255,255,255,0.6))' : 'none',
      }}
    >
      <path d={d} fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  );
}

export function SimulatorUI({ tenant, onViewOutput }: { tenant: string; onViewOutput: (v?: string) => void }) {
  const [vehicles, setVehicles] = useState(10);
  const [scenario, setScenario] = useState('mixed');
  const [duplicate, setDuplicate] = useState(0);
  const [late, setLate] = useState(0);
  const [drift, setDrift] = useState(0);
  const [runId, setRunId] = useState<string | null>(null);
  const [status, setStatus] = useState('');
  const [progress, setProgress] = useState(0);
  const [err, setErr] = useState('');
  const [history, setHistory] = useState<Run[]>(() => {
    try { return JSON.parse(localStorage.getItem('tide-runs') ?? '[]'); } catch { return []; }
  });

  function cliCommand() {
    return (
      `tide simulate --vehicles ${vehicles} --scenario ${scenario}` +
      (duplicate ? ` --duplicate-events ${duplicate}` : '') +
      (late ? ` --late-events ${late}` : '') +
      (drift ? ` --gps-drift ${drift}` : '')
    );
  }

  const vehiclesInvalid = !Number.isFinite(vehicles) || vehicles < 1 || vehicles > 5000;
  const faultLoad = Math.min(1, (duplicate * 2 + late * 2 + Math.min(1, drift / 300)) / 3);

  async function poll(id: string) {
    for (;;) {
      await new Promise((r) => setTimeout(r, 500));
      const res = await fetch(`/v1/simulate/${id}`);
      if (!res.ok) {
        setErr(`Run ${id} Stopped Unexpectedly (HTTP ${res.status}). Existing Data Is Safe. Select Start Run to Retry.`);
        setRunId(null);
        return;
      }
      const st = await res.json();
      const done = st.points ? Math.min(1, (st.accepted ?? 0) / st.points) : 0;
      setProgress(done);
      setStatus(`${st.state}: ${st.accepted ?? 0}/${st.points ?? 0} points, ${st.events ?? 0} events`);
      if (st.state !== 'running') {
        setRunId(null);
        setProgress(1);
        const run = { id, scenario, vehicles, at: new Date().toLocaleTimeString(), result: `${st.events ?? 0} events` };
        setHistory((prev) => {
          const next = [run, ...prev].slice(0, 8);
          try { localStorage.setItem('tide-runs', JSON.stringify(next)); } catch { /* ignore */ }
          return next;
        });
        return;
      }
    }
  }

  async function start() {
    if (vehiclesInvalid) { setErr('Vehicle Count Must Be Between 1 and 5000.'); return; }
    setErr('');
    setProgress(0);
    setStatus('Starting Run');
    const res = await fetch('/v1/simulate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        scenario,
        vehicles,
        tenant,
        faults: { duplicateRate: duplicate, lateRate: late, gpsDriftM: drift },
      }),
    });
    if (!res.ok) {
      setErr(`Could Not Start Run: HTTP ${res.status}. Verify the API Is Running on Port 8080. (${await res.text()})`);
      setStatus('');
      return;
    }
    const st = await res.json();
    setRunId(st.id);
    void poll(st.id);
  }

  async function stop() {
    if (!runId) return;
    await fetch(`/v1/simulate/${runId}`, { method: 'DELETE' });
  }

  return (
    <div>
      <h2 className="tide-h">Simulation Runs</h2>
      <p className="tide-sub">Synthetic telemetry for Tenant “{tenant}”. FleetSim is this console's signal generator: start a run and watch the Pipeline tape respond. Completed runs emit events into the Pipeline stream.</p>

      <div className="card" style={{ marginBottom: 16 }}>
        <h3>Step 1: Traffic Scenario <span className="tag">FleetSim</span></h3>
        <div className="scenario-grid" role="radiogroup" aria-label="Traffic Scenario">
          {SCENARIOS.map((s) => (
            <button
              key={s.key}
              role="radio"
              aria-checked={scenario === s.key}
              className={`scenario${scenario === s.key ? ' picked' : ''}`}
              onClick={() => setScenario(s.key)}
            >
              <WaveGlyph d={WAVES[s.key]} active={scenario === s.key} />
              <div className="nm">{s.key[0].toUpperCase()}{s.key.slice(1)}</div>
              <div className="ds">{s.ds}</div>
            </button>
          ))}
        </div>
        <label style={{ display: 'flex', gap: 10, alignItems: 'center', fontSize: 13, fontWeight: 600 }}>
          <span style={{ width: 110, color: 'var(--ink-2)' }}>Vehicle Count</span>
          <input className="field" type="number" min={1} max={5000} value={vehicles} onChange={(e) => setVehicles(Number(e.target.value))} style={{ width: 110 }} aria-label="Vehicle Count" />
          <span className="pill info">{vehicles} Vehicles</span>
        </label>
      </div>

      <div className="card" style={{ marginBottom: 16 }}>
        <h3>Step 2: Fault Injection <span className="tag">Fault Load {Math.round(faultLoad * 100)}%</span></h3>
        <div style={{ display: 'grid', gap: 12 }}>
          {[
            { label: 'Duplicate Packets', v: duplicate, set: setDuplicate, max: 0.5, step: 0.01, hint: 'Share of duplicated telemetry packets.' },
            { label: 'Late Arrivals', v: late, set: setLate, max: 0.5, step: 0.01, hint: 'Share of out-of-order telemetry packets.' },
            { label: 'GPS Noise (m)', v: drift, set: setDrift, max: 500, step: 10, hint: 'Positional noise in meters.' },
          ].map((f) => (
            <label key={f.label} style={{ display: 'flex', gap: 10, alignItems: 'center', fontSize: 13 }} title={f.hint}>
              <span style={{ width: 130, color: 'var(--ink-2)', fontWeight: 600 }}>{f.label}</span>
              <input type="range" min={0} max={f.max} step={f.step} value={f.v} onChange={(e) => f.set(Number(e.target.value))} style={{ flex: 1 }} aria-label={`${f.label}. ${f.hint}`} />
              <span className="pill info" style={{ minWidth: 56, textAlign: 'center' }}>{f.v}</span>
            </label>
          ))}
        </div>
      </div>

      <div className="card">
        <h3>Step 3: Execute Run <span className="tag">Live</span></h3>
        <div className="progress" role="progressbar" aria-valuenow={Math.round(progress * 100)} aria-valuemin={0} aria-valuemax={100} aria-label="Simulation Progress">
          <div style={{ width: `${progress * 100}%` }} />
        </div>
        <div style={{ display: 'flex', gap: 10, alignItems: 'center', flexWrap: 'wrap', marginTop: 12 }}>
          {runId ? (
            <button className="btn danger" onClick={stop}>Stop Run</button>
          ) : (
            <button className="btn" onClick={start} disabled={vehiclesInvalid}>Start Run</button>
          )}
          {status && <span className="pill ok">{status}</span>}
          {status.includes('events') && <button className="btn ghost" onClick={() => onViewOutput()}>View in Pipeline</button>}
        </div>
        {err && <div className="banner bad" role="alert" style={{ marginTop: 12 }}>{err}</div>}
        <h3 style={{ marginTop: 16 }}>CLI Equivalent <span className="tag">Reproducible</span></h3>
        <pre className="cli">{cliCommand()}</pre>
        <h3 style={{ marginTop: 16 }}>Run History <span className="tag">{history.length} Runs</span></h3>
        {history.length === 0 ? (
          <p className="tide-sub" style={{ margin: 0 }}>No Simulation Runs Yet in This Browser.</p>
        ) : (
          <ul style={{ listStyle: 'none', padding: 0, margin: 0, fontFamily: 'var(--font-mono)', fontSize: 12 }}>
            {history.map((r) => <li key={r.id} style={{ padding: '7px 0', borderBottom: '1px solid var(--line-soft)' }}>{r.at} · {r.scenario} × {r.vehicles} → {r.result}</li>)}
          </ul>
        )}
      </div>
    </div>
  );
}
