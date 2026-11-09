import { defineConfig, devices } from '@playwright/test';

// README のスクリーンショットを、静的なデモ (#832) から撮る (#621)。
// 普段の E2E (playwright.config.ts) とは分け、mise run //frontend:screenshots で流す。
// 撮った PNG は docs/screenshots に書き出す。main への push で CI が撮り直し、差があれば bot が PR を出す。
export default defineConfig({
  testDir: './e2e/screenshots',
  forbidOnly: process.env['CI'] !== undefined,
  reporter: 'list',
  // 画像は toHaveScreenshot の比べる先として置き、許容差を超えて変わったものと、まだないものだけを書き換える。
  snapshotPathTemplate: '../docs/screenshots/{arg}{ext}',
  updateSnapshots: 'changed',
  // Social preview は README のスクリーンショットを素材にするので、撮り終えてから合成する。
  projects: [
    { name: 'readme', testMatch: 'readme.spec.ts' },
    { name: 'social-preview', testMatch: 'social-preview.spec.ts', dependencies: ['readme'] },
  ],
  use: {
    ...devices['Desktop Chrome'],
    baseURL: 'http://127.0.0.1:4174',
    // 失敗したときは、CI が test-results を artifact に上げる (.github/workflows/screenshots.yml)。
    trace: 'retain-on-failure',
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
