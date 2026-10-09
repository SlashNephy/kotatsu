import { defineConfig, devices } from '@playwright/test';

// E2E テスト。既定では vp preview で配信したビルド済みの SPA に対して実行する。
// KOTATSU_E2E_URL を指定すると、起動済みの tenban に対して実行する (CSP の nonce は tenban が付ける)。
const baseURL = process.env['KOTATSU_E2E_URL'];

export default defineConfig({
  testDir: './e2e',
  forbidOnly: process.env['CI'] !== undefined,
  reporter: 'list',
  use: {
    baseURL: baseURL ?? 'http://127.0.0.1:4173',
    trace: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        // 手元にある Chromium を使うときに指定する。
        launchOptions: { executablePath: process.env['KOTATSU_E2E_CHROMIUM'] },
      },
    },
  ],
  webServer:
    baseURL === undefined
      ? {
          command: 'pnpm exec vp preview --host 127.0.0.1 --port 4173 --strictPort',
          url: 'http://127.0.0.1:4173',
          reuseExistingServer: false,
        }
      : undefined,
});
