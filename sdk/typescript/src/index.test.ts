import { describe, expect, it, vi, beforeEach } from 'vitest';
import { TideClient, TideError } from './index';

// The SDK is proven twice: unit tests here (mocked transport, error paths,
// reconnect behavior) and dogfooded by the console (Playwright E2E runs the
// real client against a live API on every PR).

function okJson(body: unknown, status = 200): Response {
  return { ok: status >= 200 && status < 300, status, json: async () => body, text: async () => '' } as unknown as Response;
}

function errText(status: number, body: string): Response {
  return { ok: false, status, json: async () => ({}), text: async () => body } as unknown as Response;
}

class FakeWS {
  static instances: FakeWS[] = [];
  onopen: ((e: unknown) => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;
  onclose: ((e: unknown) => void) | null = null;
  onerror: ((e: unknown) => void) | null = null;
  closed = false;
  constructor(public url: string) {
    FakeWS.instances.push(this);
  }
  close() {
    if (this.closed) return;
    this.closed = true;
    this.onclose?.({});
  }
  serverOpen() {
    this.onopen?.({});
  }
  serverSend(data: string) {
    this.onmessage?.({ data });
  }
}

beforeEach(() => {
  FakeWS.instances = [];
});

describe('queries', () => {
  it('binds the global fetch (browsers reject detached window.fetch)', async () => {
    // Regression: capturing globalThis.fetch as a bare reference works in
    // Node (undici tolerates it) but throws "Illegal invocation" in Chromium,
    // where fetch requires its global receiver. This fake enforces the same
    // rule: this must be globalThis.
    const realFetch = globalThis.fetch;
    const browserFetch = function (this: unknown, _input: RequestInfo | URL, _init?: RequestInit): Promise<Response> {
      if (this !== globalThis) return Promise.reject(new TypeError("Failed to execute 'fetch' on 'Window': Illegal invocation"));
      return Promise.resolve(okJson([{ id: '1' }]));
    };
    (globalThis as Record<string, unknown>).fetch = browserFetch;
    try {
      const c = new TideClient({ base: 'http://x' }); // no fetchImpl: SDK must bind internally
      await expect(c.events('acme')).resolves.toEqual([{ id: '1' }]);
    } finally {
      globalThis.fetch = realFetch;
    }
  });

  it('events() scopes by tenant and parses the list', async () => {
    const fetchImpl = vi.fn(async (_url: RequestInfo | URL, _init?: RequestInit): Promise<Response> => okJson([{ id: '1' }]));
    const c = new TideClient({ base: 'http://x', fetchImpl: fetchImpl });
    const out = await c.events('acme', { type: 'incident.created' });
    expect(fetchImpl).toHaveBeenCalledOnce();
    expect(String(fetchImpl.mock.calls[0][0])).toContain('tenant=acme');
    expect(out).toEqual([{ id: '1' }]);
  });

  it('non-ok responses throw TideError with status', async () => {
    const fetchImpl = vi.fn(async (_url: RequestInfo | URL, _init?: RequestInit): Promise<Response> => errText(400, 'tenant required'));
    const c = new TideClient({ base: 'http://x', fetchImpl: fetchImpl });
    await expect(c.events('')).rejects.toMatchObject({ status: 400, operation: 'events' });
  });

  it('vehicleState encodes the id', async () => {
    const fetchImpl = vi.fn(async (_url: RequestInfo | URL, _init?: RequestInit): Promise<Response> => okJson({ vehicleId: 'a/b' }));
    const c = new TideClient({ base: 'http://x', fetchImpl: fetchImpl });
    await c.vehicleState('a/b');
    expect(String(fetchImpl.mock.calls[0][0])).toContain('a%2Fb');
  });
});

describe('ingest', () => {
  it('posts the batch envelope and returns counts', async () => {
    const fetchImpl = vi.fn(async (_url: RequestInfo | URL, _init?: RequestInit): Promise<Response> => okJson({ accepted: 2, events: 1 }, 202));
    const c = new TideClient({ base: 'http://x', fetchImpl: fetchImpl });
    const out = await c.ingest([{ id: 'a' }, { id: 'b' }]);
    const init = fetchImpl.mock.calls[0]?.[1] as RequestInit | undefined;
    expect(init?.method).toBe('POST');
    expect(JSON.parse(init?.body as string)).toEqual({ batch: [{ id: 'a' }, { id: 'b' }] });
    expect(out).toEqual({ accepted: 2, events: 1 });
  });

  it('429 surfaces as TideError (caller decides retry/backoff)', async () => {
    const fetchImpl = vi.fn(async (_url: RequestInfo | URL, _init?: RequestInit): Promise<Response> => errText(429, 'rate limit exceeded'));
    const c = new TideClient({ base: 'http://x', fetchImpl: fetchImpl });
    await expect(c.ingest([])).rejects.toMatchObject({ status: 429 });
  });
});

describe('simulate', () => {
  it('start → status → cancel round-trips the run', async () => {
    const calls: string[] = [];
    const fetchImpl = vi.fn(async (url: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
      calls.push(`${init?.method ?? 'GET'} ${url}`);
      const status = (init?.method ?? 'GET') === 'POST' ? 202 : 200;
      return okJson({ id: 'sim-1', state: 'done', accepted: 10, events: 2 }, status);
    });
    const c = new TideClient({ base: 'http://x', fetchImpl: fetchImpl });
    const started = await c.simulateStart({ scenario: 'mixed', vehicles: 2 });
    expect(started.id).toBe('sim-1');
    await c.simulateStatus('sim-1');
    await c.simulateCancel('sim-1');
    expect(calls[0].startsWith('POST')).toBe(true);
    expect(calls[1].startsWith('GET')).toBe(true);
    expect(calls[2]).toContain('DELETE');
  });
});

describe('stream', () => {
  function client() {
    return new TideClient({
      base: 'http://x',
      fetchImpl: (async (_url: RequestInfo | URL, _init?: RequestInit): Promise<Response> => okJson([])) as unknown as typeof fetch,
      wsImpl: FakeWS as unknown as typeof WebSocket,
    });
  }

  it('delivers parsed events and ignores malformed frames', async () => {
    const c = client();
    const seen: unknown[] = [];
    c.stream((e) => seen.push(e));
    const ws = FakeWS.instances[0];
    ws.serverOpen();
    ws.serverSend('not json{{{');
    ws.serverSend('{"id":"e1"}');
    expect(seen).toEqual([{ id: 'e1' }]);
  });

  it('reconnects after a drop (proves the retry path fires)', async () => {
    const c = client();
    const stop = c.stream(() => {}, { baseDelayMs: 5, maxRetries: 3 });
    expect(FakeWS.instances).toHaveLength(1);
    FakeWS.instances[0].serverOpen();
    FakeWS.instances[0].close(); // server-side drop
    await new Promise((r) => setTimeout(r, 50));
    expect(FakeWS.instances.length).toBeGreaterThan(1);
    stop();
    const n = FakeWS.instances.length;
    FakeWS.instances[n - 1].close();
    await new Promise((r) => setTimeout(r, 50));
    expect(FakeWS.instances).toHaveLength(n); // stopped: no more retries
  });

  it('derives wss:// from an https base', () => {
    const c = new TideClient({
      base: 'https://api.example.com',
      fetchImpl: (async (_url: RequestInfo | URL, _init?: RequestInit): Promise<Response> => okJson([])) as unknown as typeof fetch,
      wsImpl: FakeWS as unknown as typeof WebSocket,
    });
    c.stream(() => {});
    expect(FakeWS.instances[0].url).toBe('wss://api.example.com/v1/stream');
  });
});
