import { defineConfig, devices } from '@playwright/test'

const port = 8091
const databaseURL = process.env.DATABASE_URL ?? 'postgres://gpx:gpx@localhost:5432/gpx?sslmode=disable'

// UI tests drive the real app (Go binary, embedded build, PostGIS) in a real browser: WebKit at phone size, as on
// the iPhone, and Chromium at desktop size. Run them with `make e2e`, which builds bin/kipitrek first.
export default defineConfig({
  testDir: 'e2e',
  testMatch: '*.e2e.ts',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0, // a flaky test is a bug to fix, not to retry
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: `http://localhost:${port}`,
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'iphone', use: { ...devices['iPhone 13'] } },
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
  ],
  webServer: {
    command: '../bin/kipitrek',
    url: `http://localhost:${port}/api/health`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
    // Each test gets its own client IP (fixtures.ts), trusted as from a proxy: rate limits stay on, per test.
    env: {
      PORT: String(port),
      DATABASE_URL: databaseURL,
      WEATHER_URL: '',
      SMTP_HOST: '',
      REAL_IP_HEADER: 'X-Client-IP',
      TRUSTED_PROXIES: '127.0.0.1/32,::1/128',
    },
  },
})
