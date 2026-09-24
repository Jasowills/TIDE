import { useState } from 'react';
import { VehicleTimeline } from '../screens/VehicleTimeline';
import { VehicleDetail } from '../screens/VehicleDetail';
import { ReplayUI } from '../screens/ReplayUI';

// Vehicle inspection, re-imagined around time. The timeline plots one
// vehicle's whole history on a time axis — the natural shape for a system
// whose contract is event-time correctness — with state and rule triggers
// below it for the selected moment.
export function Inspect({ tenant, initialVehicle, onOpenReplay }: {
  tenant: string;
  initialVehicle?: string;
  onOpenReplay: (v: string) => void;
}) {
  const [vehicle, setVehicle] = useState(initialVehicle || 'sim-000');

  return (
    <div className="sec-body">
      <h2 className="tide-h">Vehicle Inspector</h2>
      <p className="tide-sub">Canonical state, transition history, and rule triggers for one vehicle in Tenant “{tenant}”. Enter a Vehicle ID to begin.</p>
      <div className="toolbar">
        <input
          className="field mono"
          value={vehicle}
          onChange={(e) => setVehicle(e.target.value)}
          aria-label="Vehicle ID"
          placeholder="Vehicle ID (e.g. sim-000)"
          style={{ width: 240 }}
          list="tide-vehicles-inspect"
        />
        <datalist id="tide-vehicles-inspect">
          {['sim-000', 'sim-001', 'sim-002'].map((v) => <option key={v} value={v} />)}
        </datalist>
      </div>
      <div className="stack">
        <VehicleTimeline tenant={tenant} vehicle={vehicle} />
        <div className="trace-grid">
          <VehicleDetail key={`vd-${vehicle}`} tenant={tenant} initialVehicle={vehicle} onOpenReplay={onOpenReplay} embedded />
          <ReplayUI key={`rp-${vehicle}`} tenant={tenant} initialVehicle={vehicle} onOpenVehicle={() => {}} embedded />
        </div>
      </div>
    </div>
  );
}
