import { readFileSync } from 'node:fs';

import { expect, test } from '@playwright/test';

// 端末に保存した録画 (#480) は、サーバーにつながらなくても、一覧と再生の画面を開ける。
// 保存の操作は tenban の録画を要るので、ここでは保存し終えた状態を OPFS に直に書き、
// Service Worker がプリキャッシュした外枠と変換のプレイヤーだけで、オフラインで再生できることを確かめる。

// stream は、静的なデモの合成の映像 (20 秒)。
const stream = readFileSync(new URL('../../demo/stream.m2ts', import.meta.url));

const meta = {
  version: 1,
  id: 'Recording:offline-e2e',
  name: 'オフラインの番組',
  serviceName: 'テスト局',
  startAt: '2026-10-31T12:00:00Z',
  endAt: '2026-10-31T12:00:20Z',
  recordedFrom: '2026-10-31T12:00:00Z',
  recordedUntil: '2026-10-31T12:00:20Z',
  gaps: [],
  size: stream.byteLength,
  jikkyo: false,
  timeZone: 'Asia/Tokyo',
  savedAt: '2026-11-01T00:00:00Z',
};

test('端末に保存した録画を、オフラインで一覧して再生できる', async ({ page, context }) => {
  await page.goto('/');
  // 外枠をプリキャッシュし終えるまで待つ。
  await page.evaluate(async () => {
    await navigator.serviceWorker.ready;
  });
  await page.evaluate(
    async ({ meta, video }) => {
      const root = await navigator.storage.getDirectory();
      const recordings = await root.getDirectoryHandle('offline-recordings', { create: true });
      const directory = await recordings.getDirectoryHandle(meta.id, { create: true });
      const write = async (name: string, data: BufferSource | string) => {
        const writable = await (
          await directory.getFileHandle(name, { create: true })
        ).createWritable();
        await writable.write(data);
        await writable.close();
      };
      await write(
        'video.m2ts',
        Uint8Array.from(atob(video), (c) => c.charCodeAt(0)),
      );
      await write('meta.json', JSON.stringify(meta));
    },
    { meta, video: stream.toString('base64') },
  );

  await context.setOffline(true);
  await page.reload();
  // オフラインでは自分のユーザーを読めないので、ヘッダーには、保存した録画へのリンクだけが出る。
  await page.getByRole('link', { name: '端末の録画' }).click();
  await expect(page.getByText('オフラインの番組')).toBeVisible();
  await page.getByRole('link', { name: '再生' }).click();

  // 変換のプレイヤーは、保存したファイルを Range で読むので、ファイル全体の長さが video の長さになる。
  const video = page.locator('video');
  await expect
    .poll(() => video.evaluate((element: HTMLVideoElement) => element.duration))
    .toBeCloseTo(20, 0);
});
