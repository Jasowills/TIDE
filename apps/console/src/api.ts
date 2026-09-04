// TIDE API client — thin surface over @tide/sdk (the single source of truth
// for API shapes). Screens import from here; the SDK owns transport, retry,
// and reconnect. Design §4: every entity surfaces correlation/causation IDs.
import {
  TideClient,
  type Connection,
  type Geofence,
  type RuleTrigger,
  type TideEvent,
  type VehicleState,
} from '@tide/sdk';

export type { Connection, Geofence, RuleTrigger, TideEvent, VehicleState };

const client = new TideClient({ base: '' });

export function fetchEvents(tenant: string, extra: Record<string, string> = {}): Promise<TideEvent[]> {
  return client.events(tenant, extra);
}

export function fetchState(vehicleId: string): Promise<VehicleState> {
  return client.vehicleState(vehicleId);
}

export function fetchConnections(): Promise<Connection[]> {
  return client.connections();
}

export function fetchGeofences(): Promise<Geofence[]> {
  return client.geofences();
}

export function fetchTriggers(): Promise<RuleTrigger[]> {
  return client.triggers();
}

export function streamEvents(onEvent: (e: TideEvent) => void): () => void {
  return client.stream(onEvent);
}
