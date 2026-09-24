import { useEffect, useState } from 'react';

// Status bar: the console's persistent readout, fixed to the viewport bottom.
// API reachability comes from the App-level poll — Live means the last poll
// succeeded, Unreachable means it did not. Nothing here is decorative.
export function StatusBar({ tenant, vehicles, events, incidents, adapters, healthy, apiOk }: {
  tenant: string;
  vehicles: number;
  events: number;
  incidents: number;
  adapters: number;
  healthy: number;
  apiOk: boolean;
}) {
  return (
    <footer className="statusbar" aria-label="Console Status">
      <span className={`st-live${apiOk ? '' : ' is-down'}`}>
        <span className="pulse" />
        {apiOk ? 'Live' : 'Unreachable'}
      </span>
      <span>Tenant <b>{tenant}</b></span>
      <span>
        <b>{vehicles}</b> Vehicles · <b>{events}</b> Events · <b>{incidents}</b> Incidents
      </span>
      <span>
        Adapters <b>{healthy}/{adapters}</b>
      </span>
      <span className="st-right">
        <span>⌘K Commands</span>
        <StatusClock />
      </span>
    </footer>
  );
}

function StatusClock() {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const t = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(t);
  }, []);
  const p = (n: number) => String(n).padStart(2, '0');
  return (
    <span aria-label="Current UTC Time">
      {p(now.getUTCHours())}:{p(now.getUTCMinutes())}:{p(now.getUTCSeconds())} UTC
    </span>
  );
}
