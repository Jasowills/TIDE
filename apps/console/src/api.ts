// TIDE API client: REST queries + WebSocket live stream.
// Design §4: every entity surfaces correlation/causation IDs — traceability
// is the differentiator, never hidden behind a modal.
export interface TideEvent {
  id: string;
  type: string;
  tenantId: string;
  vehicleId: string;
  timestamp: string;
  ruleId?: string;
  ruleVersion?: string;
  correlationId: string;
  causationId?: string;
  payload?: Record<string, unknown>;
  schemaVersion: number;
}

export interface VehicleState {
  tenantId: string;
  vehicleId: string;
  motion: string;
  presence: string;
  speedKmh?: number;
  lat: number;
  lng: number;
  lastSeen: string;
  tripId?: string;
  geofences?: string[];
}

export interface Connection {
  name: string;
  state: string;
  message?: string;
  deviceCount?: number;
  msgPerSec?: number;
  lastSeen?: string;
}

export interface Geofence {
  id: string;
  tenantId: string;
  name: string;
  polygon: { lat: number; lng: number }[];
}

export interface RuleTrigger {
  RuleID: string;
  RuleVersion: string;
  VehicleID: string;
  At: string;
  MatchedInputs: Record<string, unknown>;
  ConditionsDesc: string[];
  ActionsTaken: string[];
}

const base = '';

export async function fetchEvents(tenant: string, extra: Record<string, string> = {}): Promise<TideEvent[]> {
  const q = new URLSearchParams({ tenant, ...extra });
  const r = await fetch(`${base}/v1/events?${q}`);
  if (!r.ok) throw new Error(`events: ${r.status}`);
  return r.json();
}

export async function fetchState(vehicleId: string): Promise<VehicleState> {
  const r = await fetch(`${base}/v1/vehicles/${vehicleId}/state`);
  if (!r.ok) throw new Error(`state: ${r.status}`);
  return r.json();
}

export async function fetchConnections(): Promise<Connection[]> {
  const r = await fetch(`${base}/v1/connections`);
  if (!r.ok) throw new Error(`connections: ${r.status}`);
  return r.json();
}

export async function fetchGeofences(): Promise<Geofence[]> {
  const r = await fetch(`${base}/v1/geofences`);
  if (!r.ok) throw new Error(`geofences: ${r.status}`);
  return r.json();
}

export async function fetchTriggers(): Promise<RuleTrigger[]> {
  const r = await fetch(`${base}/v1/rules/triggers`);
  if (!r.ok) throw new Error(`triggers: ${r.status}`);
  return r.json();
}

export function streamEvents(onEvent: (e: TideEvent) => void): () => void {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  const ws = new WebSocket(`${proto}://${location.host}/v1/stream`);
  ws.onmessage = (m) => {
    try {
      onEvent(JSON.parse(m.data));
    } catch {
      /* keep-alive noise: ignore, never crash the stream */
    }
  };
  return () => ws.close();
}
