import { expect, type Page, test } from '@playwright/test';

// README に載せるスクリーンショット (#621)。画面ごとに、ライトとダークの両方を撮る。
// デモのデータは、開いた時刻を基準に作られる (src/demo/data.ts、src/demo/epg.ts) ので、時刻を固定して撮る。
// ライブ視聴は、デモの映像がテストパターンなので撮らない (#1050)。

// 番組表に、夜の番組が並ぶ時刻にする。
const fixedTime = new Date('2026-04-01T20:30:00+09:00');

// 画像は toHaveScreenshot の比べる先 (docs/screenshots) として扱い、updateSnapshots: 'changed' で書き換える
// (playwright.screenshots.config.ts)。同じ画面でも、ブラウザを起動するたびに、カードの角のアンチエイリアスが
// 数ピクセルだけ変わる。許容差の中なら書き換えないので、画面が変わらない限り PNG は変わらない。
async function capture(page: Page, name: string, scheme: string) {
  // マウスの位置で出る表示 (ホバー、操作バー) が写らないよう、何もない左下の隅に置く。
  await page.mouse.move(0, 799);
  await expect(page).toHaveScreenshot(`${name}-${scheme}.png`);
}

for (const scheme of ['light', 'dark'] as const) {
  test.describe(scheme, () => {
    test.use({ colorScheme: scheme });

    test.beforeEach(async ({ page }) => {
      await page.clock.setFixedTime(fixedTime);
    });

    test('番組表', async ({ page }) => {
      await page.goto('/guide');
      await expect(page.getByText('こたつテレビ').first()).toBeVisible();
      await expect(page.getByText(/映画「雪の日の湯たんぽ」/).first()).toBeVisible();
      await capture(page, 'guide', scheme);
    });

    test('録画の一覧', async ({ page }) => {
      await page.goto('/recordings');
      await expect(page.locator('a[href^="/recordings/Recording"]').first()).toBeVisible();
      await capture(page, 'recordings', scheme);
    });

    test('録画の詳細', async ({ page }) => {
      await page.goto('/recordings');
      await page.locator('a[href^="/recordings/Recording"]').first().click();
      await expect(page.getByText('録画の品質')).toBeVisible();
      await capture(page, 'recording', scheme);
    });

    test('ルールの編集', async ({ page }) => {
      await page.goto('/rules');
      await page.locator('a[href^="/rules/Rule"]').first().click();
      // 一致する番組は、ルールを読み込んだ後に別に問い合わせるので、並ぶまで待つ。
      await expect(page.getByText(/アニメ「こたつ探偵」 第\d+話/).first()).toBeVisible();
      await capture(page, 'rule', scheme);
    });
  });
}
