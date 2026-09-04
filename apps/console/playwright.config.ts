import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  retries: 0,
  reporter: 'list',
  use: {
    baseURL: 'http://localhost:5174',
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'console', use: { ...devices['Desktop Chrome'] } }],
  // E2E runs against the DEV server (not preview): only dev proxies
  // websockets, and the live-update test proves the WS path end to end.
  // Port 5174 avoids clashing with a compose console on 5173.
  webServer: {
    command: 'npm run dev -- --port 5174 --strictPort',
    url: 'http://localhost:5174',
    reuseExistingServer: true,
    timeout: 60_000,
  },
});
