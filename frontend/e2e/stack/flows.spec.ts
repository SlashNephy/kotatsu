// 偽のチューナーサーバーと Kotatsu を立てて、主な流れを順に確かめる (#508、設計書 11.6)。scripts/e2e-stack.sh から流す。
// 初回セットアップで作った管理者でログインしたまま、1 つのページで順に進める。
import { expect, test, type Page } from '@playwright/test';

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
