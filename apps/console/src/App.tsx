import { useState } from 'react';
import { Connections } from './screens/Connections';
import { EventExplorer } from './screens/EventExplorer';
import { LiveMap } from './screens/LiveMap';
import { Overview } from './screens/Overview';
import { ReplayUI } from './screens/ReplayUI';
import { SimulatorUI } from './screens/SimulatorUI';
import { VehicleDetail } from './screens/VehicleDetail';

// Nav IS scope enforcement (§4.3): exactly the 7 V1 screens, no dispatch tab,
// no driver HR tab, nothing implying unbuilt capabilities.
const screens = ['Overview', 'Live map', 'Vehicle detail', 'Event explorer', 'Connections', 'Replay', 'Simulator'] as const;

export function App() {
  const [screen, setScreen] = useState<(typeof screens)[number]>('Overview');
  const [tenant, setTenant] = useState('default');

  return (
    <div style={{ fontFamily: 'system-ui', padding: 16, background: '#111', color: '#ddd', minHeight: '100vh' }}>
      <header style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
        <h1>TIDE console</h1>
        <nav style={{ display: 'flex', gap: 8 }}>
          {screens.map((s) => (
            <button key={s} onClick={() => setScreen(s)} style={s === screen ? { fontWeight: 'bold' } : undefined}>
              {s}
            </button>
          ))}
        </nav>
        <label style={{ marginLeft: 'auto' }}>
          tenant <input value={tenant} onChange={(e) => setTenant(e.target.value)} />
        </label>
      </header>
      {screen === 'Overview' && <Overview tenant={tenant} />}
      {screen === 'Live map' && <LiveMap tenant={tenant} />}
      {screen === 'Vehicle detail' && <VehicleDetail tenant={tenant} />}
      {screen === 'Event explorer' && <EventExplorer tenant={tenant} />}
      {screen === 'Connections' && <Connections />}
      {screen === 'Replay' && <ReplayUI tenant={tenant} />}
      {screen === 'Simulator' && <SimulatorUI />}
    </div>
  );
}
