import path from 'node:path';

import { expect, test } from '@playwright/test';

// GitHub の Social preview に使う画像を、README のスクリーンショットから合成する (#621)。
// 撮りたての画像を素材にするため、readme.spec.ts の後に流す (playwright.screenshots.config.ts)。
// 素材はページの配信元が持たないので、架空のオリジンへの要求をファイルで返す。

const origin = 'http://social-preview.test';
const frontendDirectory = path.resolve(import.meta.dirname, '../..');
const screenshotsDirectory = path.resolve(frontendDirectory, '../docs/screenshots');
const fontsDirectory = path.join(
  frontendDirectory,
  'node_modules/@fontsource/m-plus-rounded-1c/files',
);

function font(subset: string, weight: number) {
  return path.join(fontsDirectory, `m-plus-rounded-1c-${subset}-${weight}-normal.woff2`);
}

const assets = new Map([
  ['/', path.join(import.meta.dirname, 'social-preview.html')],
  ['/icon.svg', path.join(frontendDirectory, 'public/icons/icon.svg')],
  ['/guide.png', path.join(screenshotsDirectory, 'guide-dark.png')],
  ['/live.png', path.join(screenshotsDirectory, 'live-dark.png')],
  ['/fonts/latin-500.woff2', font('latin', 500)],
  ['/fonts/latin-800.woff2', font('latin', 800)],
  ['/fonts/japanese-500.woff2', font('japanese', 500)],
]);

// GitHub が勧める Social preview の大きさ。
test.use({ viewport: { width: 1280, height: 640 } });

test('Social preview', async ({ page }) => {
  await page.route(`${origin}/**`, (route) => {
    const file = assets.get(new URL(route.request().url()).pathname);

    return file === undefined ? route.abort() : route.fulfill({ path: file });
  });
  await page.goto(`${origin}/`);

  // フォントの読み込みに失敗しても、代わりのフォントで撮れてしまうので、確かめる。
  const fontStatuses = await page.evaluate(async () => {
    await document.fonts.ready;

    return [...document.fonts].map((face) => face.status);
  });
  expect(fontStatuses).toEqual(['loaded', 'loaded', 'loaded']);

  await expect(page).toHaveScreenshot('social-preview.png');
});
