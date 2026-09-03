// @tide/sdk — minimal typed client (docs/sdk: generated clients follow openapi.yaml).
export interface TideEvent {
  id: string; type: string; tenantId: string; vehicleId: string;
  timestamp: string; ruleId?: string; ruleVersion?: string;
  correlationId: string; causationId?: string;
  payload?: Record<string, unknown>; schemaVersion: number;
}

export class TideClient {
  constructor(private base = 'http://localhost:8080') {}
  async events(tenant: string, extra: Record<string, string> = {}): Promise<TideEvent[]> {
    const q = new URLSearchParams({ tenant, ...extra });
    const r = await fetch(`${this.base}/v1/events?${q}`);
    if (!r.ok) throw new Error(`events: ${r.status}`);
    return r.json();
  }
  async ingest(batch: unknown[]): Promise<{ accepted: number; events: number }> {
    const r = await fetch(`${this.base}/v1/telemetry:batch`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ batch }),
    });
    if (r.status !== 202) throw new Error(`ingest: ${r.status}`);
    return r.json();
  }
  stream(onEvent: (e: TideEvent) => void): () => void {
    const ws = new WebSocket(`${this.base.replace('http', 'ws')}/v1/stream`);
    ws.onmessage = (m) => { try { onEvent(JSON.parse(m.data)); } catch { /* ignore */ } };
    return () => ws.close();
  }
}
