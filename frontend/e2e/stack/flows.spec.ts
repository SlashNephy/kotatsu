// 偽のチューナーサーバーと Kotatsu を立てて、主な流れを順に確かめる (#508、設計書 11.6)。scripts/e2e-stack.sh から流す。
// 初回セットアップで作った管理者でログインしたまま、1 つのページで順に進める。
import { devices, expect, test, type Page } from '@playwright/test';

import { totp } from './totp';

const admin = {
  username: 'e2e-admin',
  displayName: 'E2E の管理者',
  // zxcvbn の判定を通る、推測しにくいパスワード。テストの中でだけ使う。
  password: 'kotatsu-e2e-maple-lantern-orbit-42',
};

test.describe.configure({ mode: 'serial' });

let page: Page;

test.beforeAll(async ({ browser }, testInfo) => {
  page = await browser.newPage({ baseURL: testInfo.project.use.baseURL });
});

test.afterAll(async () => {
  await page?.close();
});

test('初回セットアップで管理者を作り、認証アプリを登録してログインする', async () => {
  const token = process.env['KOTATSU_E2E_SETUP_TOKEN'];
  expect(token, 'KOTATSU_E2E_SETUP_TOKEN must be set by scripts/e2e-stack.sh').toBeTruthy();

  await page.goto('/setup');
  await page.getByRole('textbox', { name: 'セットアップトークン', exact: true }).fill(token!);
  await page.getByRole('textbox', { name: 'ユーザー名', exact: true }).fill(admin.username);
  await page.getByRole('textbox', { name: '表示名', exact: true }).fill(admin.displayName);
  await page.getByRole('textbox', { name: 'パスワード', exact: true }).fill(admin.password);
  await page.getByRole('checkbox', { name: '免責事項に同意します' }).check();
  await page.getByRole('button', { name: '管理者を作る' }).click();
  await expect(page.getByText('管理者を作りました。')).toBeVisible();

  await page.goto('/login');
  await page.getByRole('textbox', { name: 'ユーザー名', exact: true }).fill(admin.username);
  await page.getByRole('textbox', { name: 'パスワード', exact: true }).fill(admin.password);
  await page.getByRole('button', { name: 'ログイン', exact: true }).click();

  // 第二要素がないので、その場で認証アプリを登録する。キーは、4 文字ごとに空白で区切って出す。
  await expect(page.getByText('手で入力するときのキー')).toBeVisible();
  const secret = (await page.locator('pre').first().textContent())!.replace(/\s/g, '');
  // コードは、1 桁ずつの欄 (PinInput) に入れる。最初の欄から打てば、次の欄へ進む。
  await page
    .getByRole('group', { name: '認証アプリに表示されたコード' })
    .getByRole('textbox')
    .first()
    .click();
  await page.keyboard.type(totp(secret));
  await page.getByRole('button', { name: '登録する' }).click();
  await expect(page.getByText('リカバリーコード', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '控えたので続ける' }).click();

  await expect(page.getByRole('button', { name: admin.displayName })).toBeVisible();
});

test('ルールを作り、一致する番組をプレビューで確かめる', async () => {
  await page.goto('/rules/new');
  await page.getByRole('textbox', { name: 'タイトル', exact: true }).fill('E2E のルール');
  // 予約を作って録画を始めないよう、無効にして保存する。
  await page.getByRole('switch', { name: '有効' }).click({ force: true });
  await page.getByRole('button', { name: 'キーワードを追加' }).first().click();
  await page.getByRole('textbox', { name: '含めるキーワード' }).fill('こたつテスト1 テスト番組');

  // 偽のチューナーサーバーの番組表から、一致する番組を出す。
  await expect(page.getByTestId('rule-preview')).toContainText('こたつテスト1 テスト番組');

  await page.getByRole('button', { name: '保存' }).click();
  await page.waitForURL((url) => url.pathname !== '/rules/new');
  await page.goto('/rules');
  await expect(page.getByText('E2E のルール')).toBeVisible();
});

test('ライブ視聴を始めると、偽のチューナーサーバーの映像が流れる', async () => {
  await page.goto('/live');
  await page
    .getByRole('link', { name: /こたつテスト1/ })
    .first()
    .click();
  await page.waitForURL(/\/live\/.+/);

  // ブラウザの中で変換した映像が、進んでいくことを確かめる。
  await expect
    .poll(() => page.evaluate(() => document.querySelector('video')?.currentTime ?? 0), {
      timeout: 30_000,
    })
    .toBeGreaterThan(1);
});

test('スマートフォンで、ユーザーのメニューを何度でも開ける (#1663)', async ({
  browser,
}, testInfo) => {
  // スマートフォンのブラウザは、ページが横にはみ出すとレイアウトの幅を広げる。この振る舞いは isMobile で真似る。
  // 幅は、ヘッダーのユーザーのボタンが画面に収まるよう広げる。Pixel 7 の幅では、この表示名のボタンが右にはみ出す。
  const context = await browser.newContext({
    ...devices['Pixel 7'],
    viewport: { width: 540, height: 900 },
    baseURL: testInfo.project.use.baseURL,
    locale: 'ja-JP',
    storageState: await page.context().storageState(),
  });
  try {
    const mobile = await context.newPage();
    await mobile.goto('/reservations');
    const target = mobile.getByRole('button', { name: admin.displayName });
    const menu = mobile.getByRole('menu');
    // ボタンが右にはみ出していると、直す前でも開けてしまい、このテストで不具合を見分けられない。先に、収まっていることを確かめる。
    const box = await target.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.x + box!.width).toBeLessThanOrEqual(mobile.viewportSize()!.width);

    for (let i = 0; i < 3; i++) {
      await target.tap();
      await expect(menu).toBeVisible();
      // 壊れていたときは、隠す・出すをフレームごとに繰り返したので、続くフレームのすべてで、画面に収まって見えていることを確かめる。
      const frames = await mobile.evaluate(async () => {
        const results: boolean[] = [];
        for (let frame = 0; frame < 30; frame++) {
          await new Promise((resolve) => requestAnimationFrame(resolve));
          const element = document.querySelector<HTMLElement>('[role="menu"]');
          const rect = element?.getBoundingClientRect();
          results.push(
            element !== null &&
              rect !== undefined &&
              getComputedStyle(element).display !== 'none' &&
              rect.width > 0 &&
              rect.left >= 0 &&
              rect.right <= window.visualViewport!.width,
          );
        }
        return results;
      });
      expect(frames.every(Boolean), `open #${i + 1}: ${frames.join(',')}`).toBe(true);

      await target.tap();
      await expect(menu).toBeHidden();
    }
  } finally {
    await context.close();
  }
});

test('スマートフォンの幅で、ヘッダーと画面が横にはみ出さない (#1879)', async ({
  browser,
}, testInfo) => {
  for (const width of [360, 390, 412]) {
    // スマートフォンのブラウザは、ページが横にはみ出すとレイアウトの幅を広げる。この振る舞いは isMobile で真似る。
    const context = await browser.newContext({
      ...devices['Pixel 7'],
      viewport: { width, height: 800 },
      baseURL: testInfo.project.use.baseURL,
      locale: 'ja-JP',
      storageState: await page.context().storageState(),
    });
    try {
      const mobile = await context.newPage();
      // 測る前に、画面の本文を読み終えるまで待つ。読み込み中のスピナーだけを測っても、はみ出しは分からない。
      const loaded = {
        '/': mobile.getByRole('heading', { name: '放送中の番組' }),
        '/live': mobile.getByRole('link', { name: /こたつテスト1/ }).first(),
        '/search': mobile.getByText('検索結果', { exact: true }),
        '/reservations': mobile.getByRole('heading', { name: '予約', exact: true }),
      };
      for (const [path, content] of Object.entries(loaded)) {
        await mobile.goto(path);
        await expect(mobile.getByRole('button', { name: admin.displayName })).toBeVisible();
        await expect(content).toBeVisible();
        await expect(mobile.locator('main [role="status"]')).toHaveCount(0);
        const layout = await mobile.evaluate(() => ({
          innerWidth: window.innerWidth,
          scrollWidth: document.documentElement.scrollWidth,
          // ヘッダーは固定して描くので、はみ出してもページの幅は広がらない。ヘッダーの部品は、1 つずつ画面に収まっているかを見る。
          outside: [...document.querySelectorAll('header button, header a')]
            .filter((element) => {
              const rect = element.getBoundingClientRect();
              return rect.width > 0 && (rect.left < 0 || rect.right > window.innerWidth);
            })
            .map((element) => element.getAttribute('aria-label') ?? element.textContent),
        }));
        expect(layout, `${width}px ${path}`).toEqual({
          innerWidth: width,
          scrollWidth: width,
          outside: [],
        });
      }
    } finally {
      await context.close();
    }
  }
});

// ipv4 は、IPv4 アドレスの形。
const ipv4 = /\b\d{1,3}(?:\.\d{1,3}){3}\b/;

test('ログの画面で、IP アドレスを伏せた不具合の報告を作る (#1521)', async () => {
  // 報告のフォームは開かない。誤って開いても GitHub に届かないよう、止めておく。
  await page.route('https://github.com/**', (route) => route.abort());
  await page.goto('/admin/logs');

  // 画面では、ホスト名や IP アドレスをそのまま出す。偽のチューナーサーバーのログには、Kotatsu の 127.0.0.1 が入る。
  await expect(page.getByTestId('log-lines')).toContainText('127.0.0.1');

  await page.getByRole('button', { name: 'Issue にする' }).click();
  const logs = page.getByTestId('issue-logs');
  await expect(logs).toBeVisible();
  const text = await logs.inputValue();
  expect(text).toContain('[REDACTED_IP]');
  expect(text).not.toMatch(ipv4);

  // 開く URL の logs の欄にも、IP アドレスが残っていない。
  const href = await page.getByTestId('issue-open').getAttribute('href');
  const url = new URL(href!);
  expect(url.origin + url.pathname).toBe('https://github.com/SlashNephy/kotatsu/issues/new');
  expect(url.searchParams.get('template')).toBe('bug_report.yml');
  const reported = url.searchParams.get('logs') ?? '';
  expect(reported).toContain('[REDACTED_IP]');
  expect(reported).not.toMatch(ipv4);
  expect(href!.length).toBeLessThanOrEqual(5500);

  await page.getByRole('button', { name: 'やめる' }).click();
});
