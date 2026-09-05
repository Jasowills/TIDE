// @tide/sdk — the typed client for the TIDE API. This package is the single
// source of truth for API shapes: the console consumes it (workspace
// dependency), so the Playwright suite proves this client on every run.
// Design §4: every entity surfaces correlation/causation IDs.

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

export interface SimRequest {
  scenario?: string;
  vehicles?: number;
  seed?: number;
  tenant?: string;
  faults?: Record<string, number>;
}

export type SimState = 'running' | 'done' | 'cancelled' | 'failed';

export interface SimRun {
  id: string;
  state: SimState;
  scenario?: string;
  vehicles?: number;
  points?: number;
  accepted?: number;
  events?: number;
  error?: string;
}

export interface StreamOptions {
  maxRetries?: number;
  baseDelayMs?: number;
}

/** Typed API error: status preserved, body attached for 4xx debugging. */
export class TideError extends Error {
  constructor(
    public operation: string,
    public status: number,
    public body: string,
  ) {
    super(`${operation}: ${status}${body ? ` ${body.slice(0, 200)}` : ''}`);
    this.name = 'TideError';
  }
}

type FetchImpl = typeof fetch;
type WsImpl = typeof WebSocket;

export interface ClientOptions {
  base?: string;
  fetchImpl?: FetchImpl;
  wsImpl?: WsImpl;
  /** Override for non-browser runtimes (tests, SSR). Defaults to global location. */
  getLocation?: () => { protocol: string; host: string } | undefined;
}

export class TideClient {
  private readonly base: string;
  private readonly fetchOpt?: FetchImpl;
  private readonly wsOpt?: WsImpl;
  private readonly getLocation?: () => { protocol: string; host: string } | undefined;

  constructor(opts: ClientOptions | string = {}) {
    if (typeof opts === 'string') opts = { base: opts };
    this.base = opts.base ?? 'http://localhost:8080';
    this.fetchOpt = opts.fetchImpl;
    this.wsOpt = opts.wsImpl;
    this.getLocation = opts.getLocation;
  }

  // Transports resolve lazily: constructing a client never throws for a
  // transport you never use (Node 20 has fetch but no global WebSocket).
  private getFetch(): FetchImpl {
    if (this.fetchOpt) return this.fetchOpt;
    const gf = globalThis.fetch as FetchImpl | undefined;
    // Bind: window.fetch is receiver-sensitive — a detached reference throws
    // "Illegal invocation" in browsers (Node's undici fetch tolerates it,
    // which is why unit tests passed and the browser failed).
    if (gf) return gf.bind(globalThis);
    return missing('fetch');
  }

  private getWs(): WsImpl {
    if (this.wsOpt) return this.wsOpt;
    const w = globalThis.WebSocket as unknown as WsImpl | undefined;
    if (w) return w;
    return missing('WebSocket');
  }

  private async get<T>(path: string, op: string): Promise<T> {
    const r = await this.getFetch()(`${this.base}${path}`);
    if (!r.ok) throw new TideError(op, r.status, await r.text().catch(() => ''));
    return r.json() as Promise<T>;
  }

  private async post<T>(path: string, op: string, body: unknown, okStatuses: number[]): Promise<T> {
    const r = await this.getFetch()(`${this.base}${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!okStatuses.includes(r.status)) {
      throw new TideError(op, r.status, await r.text().catch(() => ''));
    }
    return r.json() as Promise<T>;
  }

  events(tenant: string, extra: Record<string, string> = {}): Promise<TideEvent[]> {
    const q = new URLSearchParams({ tenant, ...extra });
    return this.get(`/v1/events?${q}`, 'events');
  }

  vehicleState(vehicleId: string): Promise<VehicleState> {
    return this.get(`/v1/vehicles/${encodeURIComponent(vehicleId)}/state`, 'state');
  }

  connections(): Promise<Connection[]> {
    return this.get('/v1/connections', 'connections');
  }

  geofences(): Promise<Geofence[]> {
    return this.get('/v1/geofences', 'geofences');
  }

  createGeofence(g: Geofence): Promise<Geofence> {
    return this.post('/v1/geofences', 'createGeofence', g, [200, 201]);
  }

  triggers(): Promise<RuleTrigger[]> {
    return this.get('/v1/rules/triggers', 'triggers');
  }

  ingest(batch: unknown[]): Promise<{ accepted: number; events: number }> {
    return this.post('/v1/telemetry:batch', 'ingest', { batch }, [202]);
  }

  simulateStart(req: SimRequest): Promise<SimRun> {
    return this.post('/v1/simulate', 'simulateStart', req, [202]);
  }

  simulateStatus(id: string): Promise<SimRun> {
    return this.get(`/v1/simulate/${encodeURIComponent(id)}`, 'simulateStatus');
  }

  async simulateCancel(id: string): Promise<SimRun> {
    const r = await this.getFetch()(`${this.base}/v1/simulate/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    });
    if (!r.ok) throw new TideError('simulateCancel', r.status, await r.text().catch(() => ''));
    return r.json() as Promise<SimRun>;
  }

  private streamUrl(): string {
    if (/^https?:\/\//.test(this.base)) return this.base.replace(/^http/, 'ws') + '/v1/stream';
    const loc =
      this.getLocation?.() ??
      (typeof location !== 'undefined'
        ? { protocol: location.protocol, host: location.host }
        : undefined);
    if (!loc) throw new Error('stream: relative base needs a location (pass getLocation)');
    return `${loc.protocol === 'https:' ? 'wss' : 'ws'}://${loc.host}/v1/stream`;
  }

  // A silent dead stream is the worst outcome for a live dashboard: reconnect
  // with backoff so a proxy/API restart heals without a page reload.
  stream(onEvent: (e: TideEvent) => void, opts: StreamOptions = {}): () => void {
    const maxRetries = opts.maxRetries ?? 5;
    const baseDelay = opts.baseDelayMs ?? 1000;
    const url = this.streamUrl();
    const Ws = this.getWs();
    let closed = false;
    let attempts = 0;
    let ws: InstanceType<WsImpl> | null = null;
    let timer: ReturnType<typeof setTimeout> | null = null;

    function connect() {
      if (closed) return;
      ws = new Ws(url) as InstanceType<WsImpl>;
      ws.onmessage = (m: { data: string }) => {
        try {
          onEvent(JSON.parse((m as { data: string }).data));
        } catch {
          /* keep-alive noise: ignore, never crash the stream */
        }
      };
      ws.onopen = () => {
        attempts = 0;
      };
      ws.onclose = () => {
        if (closed || attempts >= maxRetries) return;
        attempts += 1;
        timer = setTimeout(connect, Math.min(baseDelay * 2 ** attempts, 15000));
      };
      ws.onerror = () => {
        ws?.close();
      };
    }
    connect();
    return () => {
      closed = true;
      if (timer) clearTimeout(timer);
      ws?.close();
    };
  }
}

function missing(what: string): never {
  throw new Error(`@tide/sdk: no ${what} available (pass it in ClientOptions)`);
}
