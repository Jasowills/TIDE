import { useMemo, useState } from 'react';
import type { TideEvent } from '../../api';
import { getIncidents } from './model';

// Operations map: every reporting device plotted from event payloads — pure
// SVG, zero tile requests, works offline. Squares read tactical: filled for
// moving, hollow for stationary, blinking diamonds for incident vehicles.
// Selecting a node loads it into the dossier; Recenter refits the projection
// to current data (the viewport otherwise stays operator-owned).
const W = 800;
const H = 520;

interface Fix {
  lat: number;
  lng: number;
  moving: boolean;
}

function payloadFix(e: TideEvent): Fix | null {
  const p = e.payload as Record<string, number | string> | undefined;
  if (!p || typeof p['lat'] !== 'number' || typeof p['lng'] !== 'number') return null;
  return {
    lat: p['lat'] as number,
    lng: p['lng'] as number,
    moving: p['motion'] === 'moving',
  };
}

export function OpsMap({ events, selected, onSelect }: {
  events: TideEvent[];
  selected: string | null;
  onSelect: (v: string | null) => void;
}) {
  const [fitKey, setFitKey] = useState(0);

  const positions = useMemo(() => {
    const m = new Map<string, Fix>();
    for (let i = events.length - 1; i >= 0; i--) {
      const f = payloadFix(events[i]);
      if (f && !m.has(events[i].vehicleId)) m.set(events[i].vehicleId, f);
    }
    return m;
  }, [events]);

  const trails = useMemo(() => {
    const t = new Map<string, { lat: number; lng: number }[]>();
    for (let i = events.length - 1; i >= 0; i--) {
      const e = events[i];
      const f = payloadFix(e);
      if (!f) continue;
      const arr = t.get(e.vehicleId) ?? [];
      arr.push({ lat: f.lat, lng: f.lng });
      t.set(e.vehicleId, arr.slice(-14));
    }
    return t;
  }, [events]);

  const incidentIds = useMemo(
    () => new Set(getIncidents(events, 500).map((e) => e.vehicleId)),
    [events],
  );

  const bounds = useMemo(() => {
    const pts = [...positions.values()];
    if (!pts.length) return null;
    let minLat = 90;
    let maxLat = -90;
    let minLng = 180;
    let maxLng = -180;
    for (const p of pts) {
      minLat = Math.min(minLat, p.lat);
      maxLat = Math.max(maxLat, p.lat);
      minLng = Math.min(minLng, p.lng);
      maxLng = Math.max(maxLng, p.lng);
    }
    const padLat = Math.max(0.5, (maxLat - minLat) * 0.25);
    const padLng = Math.max(0.5, (maxLng - minLng) * 0.25);
    return { minLat: minLat - padLat, maxLat: maxLat + padLat, minLng: minLng - padLng, maxLng: maxLng + padLng };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [positions, fitKey]);

  const ids = [...positions.keys()].sort();
  const moving = ids.filter((id) => positions.get(id)?.moving).length;

  function project(lat: number, lng: number): [number, number] {
    if (!bounds) return [W / 2, H / 2];
    const x = ((lng - bounds.minLng) / Math.max(1e-6, bounds.maxLng - bounds.minLng)) * (W - 60) + 30;
    const y = H - 30 - ((lat - bounds.minLat) / Math.max(1e-6, bounds.maxLat - bounds.minLat)) * (H - 60);
    return [x, y];
  }

  function spanKm(): string {
    if (!bounds) return '—';
    const meanLat = ((bounds.minLat + bounds.maxLat) / 2) * (Math.PI / 180);
    const dx = (bounds.maxLng - bounds.minLng) * 111.32 * Math.cos(meanLat);
    const dy = (bounds.maxLat - bounds.minLat) * 110.57;
    const s = Math.max(dx, dy);
    return s < 10 ? `${s.toFixed(1)} km` : `${Math.round(s)} km`;
  }

  return (
    <section className="panel map-panel" aria-label="Operations Map">
      <div className="panel-h">
        <h3>Operations Map</h3>
        <span className="mono-dim">
          {ids.length} Tracked · Span {spanKm()}
        </span>
        <span className="panel-h-right">
          <button className="btn ghost sm" onClick={() => setFitKey((k) => k + 1)} disabled={ids.length === 0}>
            Recenter
          </button>
        </span>
      </div>
      <div className="opsmap" role="application" aria-label={`Operations map, ${ids.length} vehicles tracked`}>
        <svg
          viewBox={`0 0 ${W} ${H}`}
          preserveAspectRatio="xMidYMid slice"
          onClick={(e) => {
            if (e.target === e.currentTarget) onSelect(null);
          }}
        >
          {[...trails.entries()].map(([id, trail]) => {
            if (trail.length < 2) return null;
            const d = trail.map((p) => project(p.lat, p.lng).join(',')).join(' ');
            const isSel = id === selected;
            return (
              <polyline
                key={id}
                points={d}
                fill="none"
                stroke="#ffffff"
                strokeOpacity={isSel ? 0.9 : 0.28}
                strokeWidth={isSel ? 2.5 : 1.5}
              />
            );
          })}
          {ids.map((id) => {
            const p = positions.get(id)!;
            const [x, y] = project(p.lat, p.lng);
            const isSel = id === selected;
            const isInc = incidentIds.has(id);
            return (
              <g
                key={id}
                transform={`translate(${x},${y})`}
                onClick={(e) => {
                  e.stopPropagation();
                  onSelect(isSel ? null : id);
                }}
                role="button"
                aria-label={`${id}${isInc ? ', incident' : p.moving ? ', moving' : ', stationary'}`}
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') onSelect(isSel ? null : id);
                }}
                style={{ cursor: 'pointer' }}
              >
                {isSel && (
                  <g stroke="#ffffff" strokeOpacity="0.7" strokeWidth="1">
                    <line x1={-22} y1="0" x2={22} y2="0" />
                    <line x1="0" y1={-22} x2="0" y2="22" />
                  </g>
                )}
                {isInc ? (
                  <rect x="-6" y="-6" width="12" height="12" transform="rotate(45)" fill="#ffffff" className="nd-blink" />
                ) : p.moving ? (
                  <rect x="-4.5" y="-4.5" width="9" height="9" fill="#ffffff" className="nd-glow" />
                ) : (
                  <rect x="-4.5" y="-4.5" width="9" height="9" fill="none" stroke="#8f8a7e" strokeWidth="1.5" />
                )}
                <text
                  y="-14"
                  textAnchor="middle"
                  fontFamily="IBM Plex Mono, monospace"
                  fontSize="10"
                  fontWeight="600"
                  fill="#f2efe6"
                  style={{ paintOrder: 'stroke', stroke: '#000', strokeWidth: 3 }}
                >
                  {isSel ? `● ${id}` : id.replace('sim-', '')}
                </text>
              </g>
            );
          })}
          {ids.length === 0 && (
            <text
              x={W / 2}
              y={H / 2}
              textAnchor="middle"
              fontFamily="IBM Plex Mono, monospace"
              fontSize="13"
              fill="#76716a"
            >
              No Position Data. Run a Simulation to Generate Traffic.
            </text>
          )}
        </svg>
      </div>
      <div className="map-legend">
        <span>
          <i className="sw sw-move" /> Moving
        </span>
        <span>
          <i className="sw sw-idle" /> Stationary
        </span>
        <span>
          <i className="sw sw-inc" /> Incident
        </span>
        <span style={{ marginLeft: 'auto' }} className="mono-dim">
          Live Plot From Event Payloads
        </span>
      </div>
    </section>
  );
}
