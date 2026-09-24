import { useEffect, useRef, useState } from 'react';
import type { TideEvent } from '../api';

// Signal tape: the event stream drawn as a phosphor trace instead of rows.
// Time runs left to right across a trailing 90-second window; every spike is
// a real event from the log, and spike height follows the transition suffix
// contract (incidents tallest, plain observations shortest). Freezing holds
// the window edge so a moment can be inspected without losing it.
const W = 640;
const H = 132;
const PAD = 10;
const BASE = H - 20;
const WINDOW_MS = 90_000;

function spikeHeight(e: TideEvent): number {
  if (e.type.startsWith('incident.')) return 1;
  if (e.type.endsWith('.started') || e.type.endsWith('.entered')) return 0.55;
  if (e.type.endsWith('.continued')) return 0.38;
  if (e.type.endsWith('.ended') || e.type.endsWith('.exited')) return 0.28;
  return 0.16;
}

function timeOf(e: TideEvent): number {
  const t = new Date(e.timestamp).getTime();
  return Number.isNaN(t) ? -1 : t;
}

function clockLabel(t: number): string {
  const d = new Date(t);
  const p = (n: number) => String(n).padStart(2, '0');
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

export function StripChart({ events, frozen }: { events: TideEvent[]; frozen: boolean }) {
  const [, force] = useState(0);
  const frozenRef = useRef<number | null>(null);

  useEffect(() => {
    if (frozen) {
      if (frozenRef.current === null) frozenRef.current = Date.now();
      return;
    }
    frozenRef.current = null;
    const t = setInterval(() => force((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, [frozen]);

  const end = frozen ? (frozenRef.current ?? Date.now()) : Date.now();
  const start = end - WINDOW_MS;
  const onTape = events
    .filter((e) => {
      const t = timeOf(e);
      return t >= start && t <= end;
    })
    .sort((a, b) => timeOf(a) - timeOf(b));
  const incidents = onTape.filter((e) => e.type.startsWith('incident.'));

  const X = (t: number) => PAD + ((t - start) / WINDOW_MS) * (W - 2 * PAD);
  const top = (e: TideEvent) => BASE - spikeHeight(e) * (H - 44);
  let d = `M${PAD},${BASE} L${W - PAD},${BASE}`;
  if (onTape.length > 0) {
    const segs = onTape.map((e) => {
      const x = X(timeOf(e)).toFixed(1);
      const y = top(e).toFixed(1);
      return `L${x},${BASE} L${x},${y} L${x},${BASE}`;
    });
    d = `M${PAD},${BASE} ${segs.join(' ')} L${W - PAD},${BASE}`;
  }

  return (
    <div>
      <div className="tape" role="img" aria-label={onTape.length ? `${onTape.length} events on the signal tape` : 'Signal tape awaiting events'}>
        <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none">
          {[0.3, 0.55, 0.8].map((f) => (
            <line key={f} x1={PAD} x2={W - PAD} y1={H * f} y2={H * f} stroke="#ffffff" strokeOpacity="0.08" strokeWidth="1" strokeDasharray="4 4" />
          ))}
          <line x1={PAD} x2={W - PAD} y1={BASE} y2={BASE} stroke="#ffffff" strokeOpacity="0.3" strokeWidth="1" />
          <path className="tape-path" d={d} fill="none" stroke="#ffffff" strokeWidth="1.8" strokeLinejoin="round" />
          {incidents.map((e) => (
            <circle key={e.id} cx={X(timeOf(e))} cy={top(e)} r="3.5" fill="#ffffff" className="tape-blink">
              <title>{`${e.type} · ${e.vehicleId} · ${e.timestamp}`}</title>
            </circle>
          ))}
          {!frozen && <circle cx={W - PAD} cy={BASE} r="4" fill="#ffffff" className="live-dot" />}
          {onTape.length === 0 && (
            <text x={W / 2} y={H / 2 - 6} textAnchor="middle" fontFamily="IBM Plex Mono, monospace" fontSize="12" fill="#76716a">
              Awaiting Events. Run a Simulation to Generate Traffic.
            </text>
          )}
        </svg>
      </div>
      <div className="tape-foot">
        <span className="mono-dim">{clockLabel(start)} → {clockLabel(end)} · Window 90s</span>
        {frozen && <span className="pill warn">Frozen</span>}
        {!frozen && onTape.length > 0 && (
          <span className="mono-dim">{onTape.length} Events on Tape · {incidents.length} Incidents</span>
        )}
      </div>
    </div>
  );
}
