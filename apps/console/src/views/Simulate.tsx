import { SimulatorUI } from '../screens/SimulatorUI';
import { Connections } from '../screens/Connections';

// Traffic simulation. FleetSim generates synthetic telemetry (including fault
// injection) so integrators can develop and test consumers without hardware.
// Output lands in the Pipeline event stream. Adapter health sits alongside
// so a run can be correlated with adapter load.
export function Simulate({ tenant, onViewOutput }: { tenant: string; onViewOutput: (v?: string) => void }) {
  return (
    <div className="sec-body">
      <h2 className="tide-h">Traffic Simulator</h2>
      <p className="tide-sub">Generate synthetic telemetry with FleetSim, with optional fault injection. Completed runs appear in the Pipeline event stream for Tenant “{tenant}”.</p>
      <div className="split">
        <SimulatorUI tenant={tenant} onViewOutput={onViewOutput} />
        <Connections />
      </div>
    </div>
  );
}
