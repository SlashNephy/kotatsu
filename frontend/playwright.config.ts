import { defineConfig, devices } from '@playwright/test';

// E2E テスト。既定では vp preview で配信したビルド済みの SPA に対して実行する。
// KOTATSU_E2E_URL を指定すると、起動済みの tenban に対して実行する (CSP の nonce は tenban が付ける)。
const baseURL = process.env['KOTATSU_E2E_URL'];

export default defineConfig({
  testDir: './e2e/mock',
  forbidOnly: process.env['CI'] !== undefined,
  reporter: 'list',
  use: {
    baseURL: baseURL ?? 'http://127.0.0.1:4173',
    trace: 'retain-on-failure',
    // 画面の言語はブラウザの言語の設定で選ぶ (#831)。テストは日本語の文言で要素を探すので、日本語にする。
    locale: 'ja-JP',
  },
  projects: [
    {
      // spa は、ビルドした SPA だけで確かめるテスト。
      name: 'spa',
      use: {
        ...devices['Desktop Chrome'],
        // 手元にある Chromium を使うときに指定する。
        launchOptions: { executablePath: process.env['KOTATSU_E2E_CHROMIUM'] },
      },
    },
    {
      // stack は、偽のチューナーサーバーと Kotatsu を立てて、主な流れを確かめるテスト (#508)。
      // scripts/e2e-stack.sh から流す。初回セットアップから順に進めるので、1 つずつ順に流す。
      name: 'stack',
      testDir: './e2e/stack',
      fullyParallel: false,
      use: {
        ...devices['Desktop Chrome'],
        launchOptions: { executablePath: process.env['KOTATSU_E2E_CHROMIUM'] },
      },
    },
  ],
  webServer:
    baseURL === undefined
      ? {
          command: 'pnpm exec vp preview --host 127.0.0.1 --port 4173 --strictPort',
          url: 'http://127.0.0.1:4173',
          // 手元では、起動済みの vp preview をそのまま使う。
          reuseExistingServer: process.env['CI'] === undefined,
        }
      : undefined,
});
