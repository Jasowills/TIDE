import { useEffect, useRef, useState } from 'react';
import { StripChart } from '../components/StripChart';
import { EventRail } from '../components/EventRail';
import { Readout, ThroughputChart, TypeBars } from '../components/Ops';
import { AttentionQueue } from '../components/wall/AttentionQueue';
import { Roster } from '../components/wall/Roster';
import { Dossier } from '../components/wall/Dossier';
import { OpsMap } from '../components/wall/OpsMap';
import { getIncidents } from '../components/wall/model';
import { TideEvent, fetchConnections, fetchEvents, streamEvents } from '../api';

// Pipeline command wall. The top answers "what needs action" (attention
// queue), "where is everything" (operations map), and "what is this vehicle"
// (dossier) at a glance; the stream, instruments, and adapters below carry
// the full record. One pause control freezes the tape, the map, and the log
// together — pausing buffers incoming events instead of dropping them. Every
// figure derives from GET /v1/events and GET /v1/connections.
export function Pipeline({ tenant, onOpenVehicle, onOpenReplay }: {
  tenant: string;
  onOpenVehicle: (v: string) => void;
  onOpenReplay: (v: string, e?: string) => void;
}) {
  const [events, setEvents] = useState<TideEvent[]>([]);
  const [conns, setConns] = useState<{ name: string; state: string }[]>([]);
  const [paused, setPaused] = useState(false);
  const [bufferedCount, setBufferedCount] = useState(0);
  const [selected, setSelected] = useState<string | null>(null);
  const pausedRef = useRef(paused);
  pausedRef.current = paused;
  const bufferRef = useRef<TideEvent[]>([]);
  const autoRef = useRef('');

  useEffect(() => {
    fetchEvents(tenant).then((e) => setEvents(e.reverse())).catch(() => {});
    fetchConnections().then(setConns).catch(() => {});
    return streamEvents((e) => {
      if (e.tenantId !== tenant) return;
      if (pausedRef.current) {
        bufferRef.current = [e, ...bufferRef.current].slice(0, 200);
        setBufferedCount((n) => n + 1);
      } else {
        setEvents((prev) => [e, ...prev].slice(0, 400));
      }
    });
  }, [tenant]);

  useEffect(() => {
    setSelected(null);
    autoRef.current = '';
    bufferRef.current = [];
    setBufferedCount(0);
  }, [tenant]);

  function resume() {
    setEvents((prev) => [...bufferRef.current.reverse(), ...prev].slice(0, 400));
    bufferRef.current = [];
    setBufferedCount(0);
    setPaused(false);
  }

  const vehicles = new Set(events.map((e) => e.vehicleId)).size;
  const incidents = getIncidents(events, 1000).length;
  const degraded = conns.filter((c) => c.state !== 'HEALTHY' && c.state !== 'CONFIGURED');

  // The wall opens on the fire: the first incident vehicle selects itself
  // until the operator picks (or clears) a selection.
  const firstIncident = getIncidents(events, 1)[0]?.vehicleId;
  useEffect(() => {
    if (autoRef.current !== tenant && firstIncident) {
      autoRef.current = tenant;
      setSelected(firstIncident);
    }
  }, [tenant, firstIncident]);

  return (
    <div className="sec-body">
      <div className="work-head">
        <div>
          <h2 className="tide-h">Pipeline Status</h2>
          <p className="tide-sub">Live health of the telemetry pipeline for Tenant “{tenant}”. Event totals, throughput, and adapter state.</p>
        </div>
      </div>

      <div className="status-strip" role="status" aria-label="Pipeline Totals">
        <Readout label="Vehicles" value={vehicles} />
        <span className="sep" aria-hidden />
        <Readout label="Events" value={events.length} />
        <span className="sep" aria-hidden />
        <Readout label="Incidents" value={incidents} alert />
        <span className="sep" aria-hidden />
        <Readout label="Adapters" value={conns.length} />
        <span style={{ marginLeft: 'auto', display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
          {degraded.length > 0 ? (
            <span className="pill bad">Degraded: {degraded.map((d) => d.name).join(', ')}</span>
          ) : (
            <span className="pill ok">{conns.length ? 'All Adapters Healthy' : 'No Adapters Reporting'}</span>
          )}
        </span>
      </div>

      {degraded.length > 0 && (
        <div className="banner bad" role="alert" style={{ marginTop: 12 }}>
          <b>Degraded Adapters:</b> {degraded.map((d) => `${d.name} (${d.state})`).join(', ')}. Events from affected devices may be delayed or stale.
        </div>
      )}

      <div className="wall">
        <div className="wall-col wall-left">
          <AttentionQueue events={events} selected={selected} onSelect={setSelected} />
          <Roster events={events} selected={selected} onSelect={setSelected} />
        </div>
        <div className="wall-col wall-center">
          <OpsMap events={events} selected={selected} onSelect={setSelected} />
          <section className="panel" aria-label="Signal Tape">
            <div className="panel-h">
              <h3>Signal Tape</h3>
              <span className="mono-dim">{paused ? 'Frozen — Resume to Follow' : 'Live Edge'}</span>
            </div>
            <div className="panel-body">
              <StripChart events={events} frozen={paused} />
              <p className="mono-dim" style={{ margin: '10px 0 0' }}>Every Spike Is a Real Event From the Log Below. Incidents Spike Tallest.</p>
            </div>
          </section>
        </div>
        <div className="wall-col wall-right">
          <Dossier tenant={tenant} vehicle={selected} onOpenVehicle={onOpenVehicle} onOpenReplay={onOpenReplay} />
        </div>
      </div>

      <div className="flow-grid" style={{ marginTop: 16 }}>
        <section className="card" aria-label="Live Event Stream">
          <h3>Live Event Stream <span className="tag">Pausable</span></h3>
          <EventRail
            events={events}
            paused={paused}
            buffered={bufferedCount}
            onPause={() => setPaused(true)}
            onResume={resume}
            onOpenVehicle={onOpenVehicle}
            onOpenReplay={onOpenReplay}
          />
        </section>
        <div className="stack">
          <div className="card"><ThroughputChart events={events} /></div>
          <div className="card"><TypeBars events={events} /></div>
        </div>
      </div>

      <div className="card" style={{ marginTop: 16 }}>
        <h3>Adapter Status <span className="tag">{conns.length} Adapters</span></h3>
        {conns.length === 0 ? (
          <span className="mono-dim">No Adapter Data. Start the Stack With docker compose up, or Run a Simulation.</span>
        ) : (
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            {conns.map((c) => (
              <span key={c.name} style={{ display: 'inline-flex', alignItems: 'center', gap: 8, border: '1px solid var(--line-soft)', borderRadius: 999, padding: '5px 6px 5px 12px', background: 'var(--paper)' }}>
                <span className="mono-dim" style={{ fontWeight: 600 }}>{c.name}</span>
                <span className={`pill ${c.state === 'HEALTHY' ? 'ok' : c.state === 'CONFIGURED' ? 'mute' : 'bad'}`}>
                  {c.state === 'CONFIGURED' ? 'Not Connected' : c.state}
                </span>
              </span>
            ))}
          </div>
        )}
      </div>

      <div className="card" style={{ marginTop: 16 }}>
        <h3>Reproduce From the API <span className="tag">CLI</span></h3>
        <pre className="cli">curl 'http://localhost:8080/v1/events?tenant={tenant}'</pre>
      </div>
    </div>
  );
}
