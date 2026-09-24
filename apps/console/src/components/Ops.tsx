import { useEffect, useState } from 'react';
import type { TideEvent } from '../api';

// Pipeline instruments: throughput, event mix, incident summary, totals.
// Every figure derives from the visible event log, so each number can be
// reproduced with the query shown on the Pipeline section.
export function Clock() {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const t = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(t);
  }, []);
  const p = (n: number) => String(n).padStart(2, '0');
  return (
    <span className="ops-clock" aria-label="Current UTC Time">
      {p(now.getUTCHours())}:{p(now.getUTCMinutes())}:{p(now.getUTCSeconds())} UTC
    </span>
  );
}

export function ThroughputChart({ events }: { events: TideEvent[] }) {
  const [, force] = useState(0);
  useEffect(() => {
    const t = setInterval(() => force((n) => n + 1), 5000);
    return () => clearInterval(t);
  }, []);
  const N = 20;
  const span = 10 * 60 * 1000;
  const now = Date.now();
  const buckets = new Array(N).fill(0);
  for (const e of events) {
    const t = new Date(e.timestamp).getTime();
    if (Number.isNaN(t)) continue;
    const i = Math.floor(((t - (now - span)) / span) * N);
    if (i >= 0 && i < N) buckets[i]++;
  }
  const max = Math.max(1, ...buckets);
  const W = 300, H = 84, P = 4;
  const X = (i: number) => P + (i / (N - 1)) * (W - 2 * P);
  const Y = (v: number) => H - P - (v / max) * (H - 2 * P - 12);
  const line = buckets.map((v, i) => `${i === 0 ? 'M' : 'L'}${X(i).toFixed(1)},${Y(v).toFixed(1)}`).join(' ');
  const rate = buckets[N - 1] * 2;
  const [hover, setHover] = useState<number | null>(null);
  function onMove(ev: React.MouseEvent<SVGSVGElement>) {
    const r = ev.currentTarget.getBoundingClientRect();
    const i = Math.round(((ev.clientX - r.left) / r.width) * (N - 1));
    setHover(Math.max(0, Math.min(N - 1, i)));
  }
  return (
    <div className="ops-chart">
      <div className="ops-chart-head">
        <span>Throughput (Last 10 Minutes)</span>
        <b>{rate}/min</b>
      </div>
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" role="img" aria-label={`Event throughput, currently ${rate} per minute`} onMouseMove={onMove} onMouseLeave={() => setHover(null)}>
        {[0.25, 0.5, 0.75].map((f) => (
          <line key={f} x1={P} x2={W - P} y1={H * f} y2={H * f} stroke="#ffffff" strokeOpacity="0.12" strokeWidth="1" strokeDasharray="4 4" />
        ))}
        <path d={`${line} L${X(N - 1).toFixed(1)},${H - P} L${X(0).toFixed(1)},${H - P} Z`} fill="#ffffff" fillOpacity="0.1" />
        <path d={line} fill="none" stroke="#ffffff" strokeWidth="2.5" strokeLinejoin="round" />
        <circle className="live-dot" cx={X(N - 1)} cy={Y(buckets[N - 1])} r="4.5" fill="#ffffff" stroke="#0a0a0c" strokeWidth="2" />
      </svg>
      {hover !== null && (
        <span className="mono-dim">{N - 1 - hover === 0 ? 'Live Edge' : `${(N - 1 - hover) * 30}s Ago`} · {buckets[hover]} Events</span>
      )}
    </div>
  );
}

export function TypeBars({ events }: { events: TideEvent[] }) {
  const counts = new Map<string, number>();
  for (const e of events) counts.set(e.type, (counts.get(e.type) ?? 0) + 1);
  const top = [...counts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 5);
  const max = Math.max(1, ...top.map(([, n]) => n));
  return (
    <div className="ops-chart">
      <div className="ops-chart-head"><span>Events by Type</span><b>{counts.size} Types</b></div>
      {top.length === 0 && <p className="tide-sub" style={{ margin: 0 }}>No Events Recorded Yet.</p>}
      {top.map(([t, n]) => (
        <div key={t} className="ops-bar-row">
          <span className="ops-bar-label">{t}</span>
          <span className="ops-bar-track">
            <span className="ops-bar-fill" style={{ width: `${(n / max) * 100}%`, background: barColor(t) }} />
          </span>
          <b>{n}</b>
        </div>
      ))}
    </div>
  );
}

// Bar fills are monochrome textures: solid white for incidents, descending
// gray weights for the rest, hatched for unclassified types.
function barColor(t: string): string {
  if (t.startsWith('incident.')) return '#ffffff';
  if (t.endsWith('.started') || t.endsWith('.entered')) return 'rgba(255,255,255,0.62)';
  if (t.endsWith('.ended') || t.endsWith('.exited')) return 'rgba(255,255,255,0.34)';
  return 'repeating-linear-gradient(45deg,#55555e 0 4px,#2c2c34 4px 8px)';
}

export function Readout({ label, value, alert }: { label: string; value: number; alert?: boolean }) {
  return (
    <div className="stat-block" key={`${label}-${value}`}>
      <div className={`stat-v${alert && value > 0 ? ' alert' : ''}`}>{value}</div>
      <div className="stat-l">{label}</div>
    </div>
  );
}
