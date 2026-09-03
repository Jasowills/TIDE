import { useState } from 'react';

// T106 Simulator UI — vehicle count, scenario, rate, fault injection sliders,
// start/stop. Drives `tide simulate` flags; output lands in the event stream.
export function SimulatorUI() {
  const [vehicles, setVehicles] = useState(10);
  const [scenario, setScenario] = useState('mixed');
  const [duplicate, setDuplicate] = useState(0);
  const [late, setLate] = useState(0);
  const [drift, setDrift] = useState(0);
  const [cmd, setCmd] = useState('');

  function build() {
    setCmd(
      `tide simulate --vehicles ${vehicles} --scenario ${scenario}` +
      (duplicate ? ` --duplicate-events ${duplicate}` : '') +
      (late ? ` --late-events ${late}` : '') +
      (drift ? ` --gps-drift ${drift}` : ''),
    );
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
      <button onClick={build}>Build command</button>
      {cmd && <pre>{cmd}</pre>}
      <p>Run it against a live <code>tide-api</code>; watch results in Event explorer.</p>
    </div>
  );
}
