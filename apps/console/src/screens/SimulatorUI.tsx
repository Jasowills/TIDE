import { useState } from 'react';

// T106 Simulator UI — vehicle count, scenario, fault injection sliders,
// Start/Stop. Start executes FleetSim server-side through the production
// pipeline (POST /v1/simulate); output lands live in Event explorer.
// The equivalent CLI command stays visible for copy-paste runs.
export function SimulatorUI() {
  const [vehicles, setVehicles] = useState(10);
  const [scenario, setScenario] = useState('mixed');
  const [duplicate, setDuplicate] = useState(0);
  const [late, setLate] = useState(0);
  const [drift, setDrift] = useState(0);
  const [runId, setRunId] = useState<string | null>(null);
  const [status, setStatus] = useState('');
  const [err, setErr] = useState('');

  function cliCommand() {
    return (
      `tide simulate --vehicles ${vehicles} --scenario ${scenario}` +
      (duplicate ? ` --duplicate-events ${duplicate}` : '') +
      (late ? ` --late-events ${late}` : '') +
      (drift ? ` --gps-drift ${drift}` : '')
    );
  }

  async function poll(id: string) {
    for (;;) {
      await new Promise((r) => setTimeout(r, 500));
      const res = await fetch(`/v1/simulate/${id}`);
      if (!res.ok) {
        setErr(`run ${id} lost (${res.status})`);
        setRunId(null);
        return;
      }
      const st = await res.json();
      setStatus(`${st.state}: ${st.accepted ?? 0}/${st.points ?? 0} points, ${st.events ?? 0} events`);
      if (st.state !== 'running') {
        setRunId(null);
        return;
      }
    }
  }

  async function start() {
    setErr('');
    setStatus('starting…');
    const res = await fetch('/v1/simulate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        scenario,
        vehicles,
        tenant: 'default',
        faults: { duplicateRate: duplicate, lateRate: late, gpsDriftM: drift },
      }),
    });
    if (!res.ok) {
      setErr(`start failed: ${res.status} ${await res.text()}`);
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
      <h2>Simulator</h2>
      <label>vehicles <input type="number" value={vehicles} onChange={(e) => setVehicles(Number(e.target.value))} /></label>
      <label> scenario
        <select value={scenario} onChange={(e) => setScenario(e.target.value)}>
          {['normal', 'speeding', 'idling', 'offline', 'mixed'].map((s) => <option key={s}>{s}</option>)}
        </select>
      </label>
      <label> duplicate <input type="range" min={0} max={0.5} step={0.01} value={duplicate} onChange={(e) => setDuplicate(Number(e.target.value))} /> {duplicate}</label>
      <label> late <input type="range" min={0} max={0.5} step={0.01} value={late} onChange={(e) => setLate(Number(e.target.value))} /> {late}</label>
      <label> gps drift (m) <input type="range" min={0} max={500} step={10} value={drift} onChange={(e) => setDrift(Number(e.target.value))} /> {drift}</label>
      {runId ? (
        <button onClick={stop}>Stop</button>
      ) : (
        <button onClick={start}>Start</button>
      )}
      {status && <p>{status}</p>}
      {err && <p style={{ color: 'red' }}>{err}</p>}
      <pre>{cliCommand()}</pre>
      <p>Output lands live in Event explorer (same pipeline as real adapters).</p>
    </div>
  );
}
