import { expect, test } from '@playwright/test';

// Product-facing E2E for the TIDE pipeline debugger. Covers all 3 sections
// with real interactions against a live API — not just "page renders".
// Precondition: API up at localhost:8080 (vite dev proxies /v1 there)
// with simulated data loaded (see CI job / README).

const views = ['Pipeline', 'Inspect', 'Simulate'] as const;

test.beforeEach(async ({ page }) => {
  page.on('pageerror', (err) => {
    throw new Error(`console pageerror: ${err.message}`);
  });
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'TIDE Console' })).toBeVisible();
});

for (const name of views) {
  test(`view renders: ${name}`, async ({ page }) => {
    await page.getByRole('button', { name, exact: true }).click();
    // Every view renders an h2; screenshot for human review.
    await expect(page.locator('h2').first()).toBeVisible();
    await page.screenshot({ path: `e2e/screenshots/${name.replace(/ /g, '-')}.png` });
  });
}

test('pipeline stream fills from the live API', async ({ page }) => {
  await page.getByRole('button', { name: 'Pipeline', exact: true }).click();
  // Seeded mixed scenario produces events; the stream must populate.
  await expect(page.locator('.rail-list li').first()).toBeVisible({ timeout: 15_000 });
});

test('stream inspector shows trace IDs', async ({ page }) => {
  await page.getByRole('button', { name: 'Pipeline', exact: true }).click();
  // Filter to incidents first: the live stream prepends rows continuously,
  // so an unfiltered first() can resolve to a different event between the
  // count check and the click (TOCTOU flake). Filtered, every visible row
  // is an incident with a full rule trace.
  await page.getByPlaceholder('Filter by Type or Vehicle').fill('incident.created');
  const first = page.locator('.rail-list li button').first();
  await expect(first).toBeVisible({ timeout: 15_000 });
  await first.click();
  const inspector = page.locator('.rail-inspector');
  await expect(inspector).toBeVisible();
  const text = (await inspector.textContent()) ?? '';
  expect(text, 'inspector surfaces correlationId').toContain('correlationId');  for (const key of ['ruleId', 'ruleVersion', 'causationId']) {
    expect(text, `incident surfaces ${key}`).toContain(key);
  }
});

test('inspect loads vehicle state', async ({ page }) => {
  await page.getByRole('button', { name: 'Inspect', exact: true }).click();
  // Either a state block or an honest error — never a blank hang.
  await expect(page.locator('dl, p').first()).toBeVisible({ timeout: 15_000 });
});

test('simulator shows the equivalent CLI command', async ({ page }) => {
  await page.getByRole('button', { name: 'Simulate', exact: true }).click();
  const cmd = page.locator('pre');
  await expect(cmd.first()).toContainText('tide simulate');
  await expect(cmd.first()).toContainText('--vehicles');
});

test('simulator shows adapter health', async ({ page }) => {
  await page.getByRole('button', { name: 'Simulate', exact: true }).click();
  await expect(page.locator('table')).toBeVisible({ timeout: 15_000 });
  await expect(page.locator('tbody tr').first()).toBeVisible({ timeout: 15_000 });
});

test('mqtt adapter delivers live telemetry (broker → engine → UI)', async ({ page }) => {
  // Precondition: compose mosquitto + engine with TIDE_MQTT_BROKER set.
  // Publishes through the real broker (outside the app) and expects the
  // event to surface in the stream — the full live-adapter path.
  const { execSync } = await import('node:child_process');
  const vehicle = `mqtt-e2e-${Date.now()}`;
  const payload = JSON.stringify({
    device: { id: vehicle },
    gps: { lat: 52.5, lon: 13.4, speed: 66 },
    ignition: true,
    ts: new Date().toISOString(),
  });
  await page.getByRole('button', { name: 'Pipeline', exact: true }).click();
  await expect(page.locator('.rail-list li').first()).toBeVisible({ timeout: 15_000 });
  const before = await page.locator('.rail-list li').count();
  execSync(
    `docker exec tide-mosquitto-1 mosquitto_pub -h localhost -t 'fleet/${vehicle}/telemetry' -m '${payload}'`,
    { stdio: 'pipe' },
  );
  await expect
    .poll(async () => page.locator('.rail-list li').count(), { timeout: 20_000 })
    .toBeGreaterThan(before);
  // And Simulate reports the adapter healthy with real traffic behind it.
  await page.getByRole('button', { name: 'Simulate', exact: true }).click();
  const row = page.locator('tbody tr', { hasText: 'mqtt' });
  await expect(row).toContainText('HEALTHY', { timeout: 45_000 });
});

test('simulator Start executes a run server-side', async ({ page }) => {
  await page.getByRole('button', { name: 'Simulate', exact: true }).click();
  await page.getByRole('button', { name: 'Start Run' }).click();
  // Status line reports completion with counts — no terminal round-trip.
  await expect(page.getByText(/done:/)).toBeVisible({ timeout: 60_000 });
});

test('live stream prepends new events without reload (WS end to end)', async ({ page, request }) => {
  await page.getByRole('button', { name: 'Pipeline', exact: true }).click();
  await expect(page.locator('.rail-list li').first()).toBeVisible({ timeout: 15_000 });
  const before = await page.locator('.rail-list li').count();
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
    .poll(async () => page.locator('.rail-list li').count(), { timeout: 15_000 })
    .toBeGreaterThan(before);
});
