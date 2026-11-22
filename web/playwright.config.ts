import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  use: {
    baseURL: 'http://127.0.0.1:4174',
    trace: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: [
    {
      command: 'node e2e/mock-admin.mjs',
      url: 'http://127.0.0.1:18089/healthz',
      reuseExistingServer: !process.env.CI,
    },
    {
      command: 'node server/serve.mjs',
      env: { PORT: '4174' },
      url: 'http://127.0.0.1:4174/__af/__ping',
      reuseExistingServer: !process.env.CI,
    },
  ],
})
