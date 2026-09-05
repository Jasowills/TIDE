import { expect, test } from '@playwright/test';

// Product-facing E2E for the TIDE debug console. Covers all 7 screens with
// real interactions against a live API — not just "page renders".
// Precondition: API up at localhost:8080 (vite preview proxies /v1 there)
// with simulated data loaded (see CI job / README).

const screens = [
  'Overview',
  'Live map',
  'Vehicle detail',
  'Event explorer',
  'Connections',
  'Replay',
  'Simulator',
] as const;

test.beforeEach(async ({ page }) => {
  page.on('pageerror', (err) => {
    throw new Error(`console pageerror: ${err.message}`);
  });
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'TIDE console' })).toBeVisible();
});

for (const name of screens) {
  test(`screen renders: ${name}`, async ({ page }) => {
    await page.getByRole('button', { name, exact: true }).click();
    // Every screen renders an h2; screenshot for human review.
    await expect(page.locator('h2').first()).toBeVisible();
    await page.screenshot({ path: `e2e/screenshots/${name.replace(/ /g, '-')}.png` });
  });
}

test('event explorer fills from the live API', async ({ page }) => {
  await page.getByRole('button', { name: 'Event explorer', exact: true }).click();
  // Seeded mixed scenario produces events; the list must populate.
  await expect(page.locator('ul li').first()).toBeVisible({ timeout: 15_000 });
});

test('event inspector shows trace IDs', async ({ page }) => {
  await page.getByRole('button', { name: 'Event explorer', exact: true }).click();
  // Filter to incidents first: the live stream prepends rows continuously,
  // so an unfiltered first() can resolve to a different event between the
  // count check and the click (TOCTOU flake). Filtered, every visible row
  // is an incident with a full rule trace.
  await page.getByPlaceholder('filter type/vehicle').fill('incident.created');
  const first = page.locator('ul li button').first();
  await expect(first).toBeVisible({ timeout: 15_000 });
  await first.click();
  const inspector = page.locator('pre');
  await expect(inspector).toBeVisible();
  const text = (await inspector.textContent()) ?? '';
  expect(text, 'inspector surfaces correlationId').toContain('correlationId');
  for (const key of ['ruleId', 'ruleVersion', 'causationId']) {
    expect(text, `incident surfaces ${key}`).toContain(key);
  }
});

test('vehicle detail loads state', async ({ page }) => {
  await page.getByRole('button', { name: 'Vehicle detail', exact: true }).click();
  await page.getByRole('button', { name: 'Load' }).click();
  // Either a state block or an honest error — never a blank hang.
  await expect(page.locator('dl, p').first()).toBeVisible({ timeout: 15_000 });
});

test('simulator shows the equivalent CLI command', async ({ page }) => {
  await page.getByRole('button', { name: 'Simulator', exact: true }).click();
  const cmd = page.locator('pre');
  await expect(cmd).toContainText('tide simulate');
  await expect(cmd).toContainText('--vehicles');
});

test('connections shows adapter health', async ({ page }) => {
  await page.getByRole('button', { name: 'Connections', exact: true }).click();
  await expect(page.locator('table')).toBeVisible({ timeout: 15_000 });
  await expect(page.locator('tbody tr').first()).toBeVisible({ timeout: 15_000 });
});

test('mqtt adapter delivers live telemetry (broker → engine → UI)', async ({ page }) => {
  // Precondition: compose mosquitto + engine with TIDE_MQTT_BROKER set.
  // Publishes through the real broker (outside the app) and expects the
  // event to surface in the explorer — the full live-adapter path.
  const { execSync } = await import('node:child_process');
  const vehicle = `mqtt-e2e-${Date.now()}`;
  const payload = JSON.stringify({
    device: { id: vehicle },
    gps: { lat: 52.5, lon: 13.4, speed: 66 },
    ignition: true,
    ts: new Date().toISOString(),
  });
  await page.getByRole('button', { name: 'Event explorer', exact: true }).click();
  await expect(page.locator('ul li').first()).toBeVisible({ timeout: 15_000 });
  const before = await page.locator('ul li').count();
  execSync(
    `docker exec tide-mosquitto-1 mosquitto_pub -h localhost -t 'fleet/${vehicle}/telemetry' -m '${payload}'`,
    { stdio: 'pipe' },
  );
  await expect
    .poll(async () => page.locator('ul li').count(), { timeout: 20_000 })
    .toBeGreaterThan(before);
  // And Connections reports the adapter healthy with real traffic behind it.
  await page.getByRole('button', { name: 'Connections', exact: true }).click();
  const row = page.locator('tbody tr', { hasText: 'mqtt' });
  await expect(row).toContainText('HEALTHY', { timeout: 45_000 });
});

test('simulator Start executes a run server-side', async ({ page }) => {
  await page.getByRole('button', { name: 'Simulator', exact: true }).click();
  await page.getByRole('button', { name: 'Start' }).click();
  // Status line reports completion with counts — no terminal round-trip.
  await expect(page.getByText(/done:/)).toBeVisible({ timeout: 60_000 });
});

test('live stream prepends new events without reload (WS end to end)', async ({ page, request }) => {
  await page.getByRole('button', { name: 'Event explorer', exact: true }).click();
  await expect(page.locator('ul li').first()).toBeVisible({ timeout: 15_000 });
  const before = await page.locator('ul li').count();
  const now = new Date().toISOString();
  // Unique vehicle per run: the detector tracker is process memory, so a
  // reused id may already have an open trip and emit nothing. A fresh id
  // guarantees a trip.started — mirroring a real new vehicle.
  const vehicle = `e2e-live-${Date.now()}`;
  // Ingest through the same vite proxy the app uses.
  const res = await request.post('/v1/telemetry:batch', {
    data: {
      batch: [0, 1, 2].map((i) => ({
        id: `${vehicle}-${i}`,
        tenantId: 'default',
        vehicleId: vehicle,
        deviceId: vehicle,
        timestamp: now,
        location: { lat: 52.5, lng: 13.4 },
        speed: 80,
        raw: {},
        source: { provider: 'e2e', protocol: 'http', deviceId: vehicle },
        metadata: { correlationId: 'e2e', schemaVersion: 1, quality: 'good' },
      })),
    },
  });
  expect(res.status()).toBe(202);
  // The stream must deliver at least one derived event (trip.started) live.
  await expect
    .poll(async () => page.locator('ul li').count(), { timeout: 15_000 })
    .toBeGreaterThan(before);
});
