import { useEffect, useState } from 'react';
import {
  RuleTrigger,
  TideEvent,
  VehicleState,
  fetchEvents,
  fetchState,
  fetchTriggers,
} from '../../api';
import { getIncidents, timeAgo } from './model';

// Vehicle dossier: the selected vehicle at a glance — canonical state, key
// figures, the latest rule trigger, and recent events. Full history and rule
// comparison live one scroll away in Inspect; the buttons there jump to them.
export function Dossier({ tenant, vehicle, onOpenVehicle, onOpenReplay }: {
  tenant: string;
  vehicle: string | null;
  onOpenVehicle: (v: string) => void;
  onOpenReplay: (v: string) => void;
}) {
  if (!vehicle) {
    return (
      <section className="panel" aria-label="Vehicle Dossier">
        <div className="panel-h">
          <h3>Vehicle Dossier</h3>
        </div>
        <p className="panel-empty">
          No Vehicle Selected. Pick One From the Queue, the Roster, or the Map to Inspect Its State and Rule Trace.
        </p>
      </section>
    );
  }
  return <DossierBody key={vehicle} tenant={tenant} vehicle={vehicle} onOpenVehicle={onOpenVehicle} onOpenReplay={onOpenReplay} />;
}

function DossierBody({ tenant, vehicle, onOpenVehicle, onOpenReplay }: {
  tenant: string;
  vehicle: string;
  onOpenVehicle: (v: string) => void;
  onOpenReplay: (v: string) => void;
}) {
  const [state, setState] = useState<VehicleState | null>(null);
  const [events, setEvents] = useState<TideEvent[]>([]);
  const [trigger, setTrigger] = useState<RuleTrigger | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let dead = false;
    setState(null);
    setEvents([]);
    setTrigger(null);
    setFailed(false);
    fetchState(vehicle)
      .then((s) => {
        if (!dead) setState(s);
      })
      .catch(() => {
        if (!dead) setFailed(true);
      });
    fetchEvents(tenant, { vehicle })
      .then((e) => {
        if (!dead) setEvents(e.slice(0, 6));
      })
      .catch(() => {
        if (!dead) setFailed(true);
      });
    fetchTriggers()
      .then((all) => {
        if (dead) return;
        const match =
          all.find((t) => t.VehicleID === vehicle && t.ConditionsDesc.length > 0) ??
          all.find((t) => t.VehicleID === vehicle) ??
          null;
        setTrigger(match);
      })
      .catch(() => {
        /* Triggers are supplementary; the dossier stands without them. */
      });
    return () => {
      dead = true;
    };
  }, [tenant, vehicle]);

  const incidentOpen = events.some((e) => e.type.startsWith('incident.'));
  const lastSeen = state ? +new Date(state.lastSeen) : -1;

  return (
    <section className="panel" aria-label={`Dossier for ${vehicle}`}>
      <div className="panel-h">
        <h3 className="mono">{vehicle}</h3>
        {incidentOpen && <span className="pill bad">Incident</span>}
      </div>
      <div className="panel-body">
        {failed && !state && (
          <div className="banner bad" role="alert">
            Could Not Load Vehicle “{vehicle}”. Verify the API Is Running.
          </div>
        )}
        {!state && !failed ? (
          <div style={{ display: 'grid', gap: 8 }} aria-label="Loading Dossier">
            <div className="shimmer" style={{ height: 14 }} />
            <div className="shimmer" style={{ height: 14, width: '70%' }} />
            <div className="shimmer" style={{ height: 14, width: '85%' }} />
          </div>
        ) : (
          state && (
            <>
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                <span className={`pill ${state.motion === 'moving' ? 'info' : 'mute'}`}>{state.motion}</span>
                <span className="pill ok">{state.presence}</span>
                {state.tripId && <span className="pill rule">Trip {state.tripId}</span>}
              </div>
              <div className="stat-grid">
                <div className="stat-cell">
                  <div className="k">Speed</div>
                  <div className="v">{state.speedKmh ?? '—'} km/h</div>
                </div>
                <div className="stat-cell">
                  <div className="k">Last Seen</div>
                  <div className="v" title={state.lastSeen}>{timeAgo(Number.isNaN(lastSeen) ? -1 : lastSeen)}</div>
                </div>
                <div className="stat-cell">
                  <div className="k">Position</div>
                  <div className="v">
                    {state.lat}, {state.lng}
                  </div>
                </div>
                <div className="stat-cell">
                  <div className="k">Events</div>
                  <div className="v">{getIncidents(events, 1000).length} Incidents</div>
                </div>
              </div>
            </>
          )
        )}
        <div className="stat-cell" style={{ marginBottom: 8 }}>
          <div className="k">Latest Rule Trigger</div>
          <div className="v">
            {trigger
              ? `${trigger.RuleID}/${trigger.RuleVersion} · ${trigger.ConditionsDesc.length} Conditions`
              : 'No Rule Triggers Recorded'}
          </div>
        </div>
        {events.length > 0 && (
          <ul className="mini-list">
            {events.map((e) => (
              <li key={e.id}>
                <span style={{ color: 'var(--faint)' }}>{timeAgo(+new Date(e.timestamp))}</span>
                <span>{e.type}</span>
              </li>
            ))}
          </ul>
        )}
        <div className="toolbar" style={{ margin: '12px 0 0' }}>
          <button className="btn ghost sm" onClick={() => onOpenVehicle(vehicle)}>
            Full Trace
          </button>
          <button className="btn dark sm" onClick={() => onOpenReplay(vehicle)}>
            Rule History
          </button>
        </div>
      </div>
    </section>
  );
}
