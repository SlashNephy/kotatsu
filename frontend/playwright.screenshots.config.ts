import { defineConfig, devices } from '@playwright/test';

// README のスクリーンショットを、静的なデモ (#832) から撮る (#621)。
// 普段の E2E (playwright.config.ts) とは分け、mise run //frontend:screenshots で流す。
// 撮った PNG は docs/screenshots に書き出す。main への push で CI が撮り直し、差があれば bot が PR を出す。
export default defineConfig({
  testDir: './e2e/screenshots',
  forbidOnly: process.env['CI'] !== undefined,
  reporter: 'list',
  use: {
    ...devices['Desktop Chrome'],
    baseURL: 'http://127.0.0.1:4174',
    viewport: { width: 1280, height: 800 },
    deviceScaleFactor: 1,
    locale: 'ja-JP',
    timezoneId: 'Asia/Tokyo',
    // 手元にある Chromium を使うときに指定する。
    launchOptions: { executablePath: process.env['KOTATSU_E2E_CHROMIUM'] },
  },
  webServer: {
    command: 'pnpm exec vp preview --outDir dist/demo --host 127.0.0.1 --port 4174 --strictPort',
    url: 'http://127.0.0.1:4174',
    reuseExistingServer: process.env['CI'] === undefined,
  },
});
