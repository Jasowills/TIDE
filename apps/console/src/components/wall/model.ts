import type { TideEvent } from '../../api';

// Shared derivations for the operations wall. Everything here is computed
// from the event log the console already holds — no synthetic data.

export type Severity = 'inc' | 'start' | 'end' | 'info';

export function severityOf(type: string): Severity {
  if (type.startsWith('incident.')) return 'inc';
  if (type.endsWith('.started') || type.endsWith('.entered')) return 'start';
  if (type.endsWith('.ended') || type.endsWith('.exited')) return 'end';
  return 'info';
}

export function getIncidents(events: TideEvent[], limit = 40): TideEvent[] {
  return events.filter((e) => e.type.startsWith('incident.')).slice(0, limit);
}

export interface RosterRow {
  id: string;
  lastType: string;
  lastAt: number; // epoch ms, -1 when the timestamp is unusable
  severity: Severity;
  eventCount: number;
}

export function getRoster(events: TideEvent[]): RosterRow[] {
  const by = new Map<string, TideEvent[]>();
  for (const e of events) {
    const arr = by.get(e.vehicleId);
    if (arr) arr.push(e);
    else by.set(e.vehicleId, [e]);
  }
  const rows: RosterRow[] = [...by.entries()].map(([id, list]) => {
    const latest = [...list].sort((a, b) => +new Date(b.timestamp) - +new Date(a.timestamp))[0];
    const t = +new Date(latest.timestamp);
    const severity: Severity = list.some((e) => e.type.startsWith('incident.')) ? 'inc' : severityOf(latest.type);
    return { id, lastType: latest.type, lastAt: Number.isNaN(t) ? -1 : t, severity, eventCount: list.length };
  });
  const rank = (s: Severity) => (s === 'inc' ? 0 : s === 'start' ? 1 : 2);
  return rows.sort(
    (a, b) => rank(a.severity) - rank(b.severity) || b.lastAt - a.lastAt || a.id.localeCompare(b.id),
  );
}

export function timeAgo(t: number): string {
  if (t < 0) return '—';
  const s = Math.max(0, Math.round((Date.now() - t) / 1000));
  if (s < 5) return 'now';
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  return new Date(t).toLocaleDateString();
}

export function clockOf(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(+d) ? iso : d.toLocaleTimeString();
}
