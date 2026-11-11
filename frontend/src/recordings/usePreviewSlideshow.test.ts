import { afterEach, describe, expect, test, vi } from 'vitest';

import { loadManifest } from './usePreviewSlideshow';

const manifest = { times: [0, 10], columns: 2, width: 160, height: 90 };

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('loadManifest (#1424)', () => {
  test.each([
    ['503', () => Promise.resolve(new Response('', { status: 503 }))],
    ['通信の失敗', () => Promise.reject(new TypeError('Failed to fetch'))],
  ])('%s で読めなかったら、次に読むときに取り直す', async (_, fail) => {
    const previewUrl = `/media/recordings/${crypto.randomUUID()}/preview`;
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockImplementationOnce(fail)
      .mockResolvedValueOnce(Response.json(manifest));
    vi.stubGlobal('fetch', fetchMock);

    expect(await loadManifest(previewUrl)).toBeNull();
    expect(await loadManifest(previewUrl)).toEqual(manifest);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  test('プレビュー画像がなければ (404)、取り直さずに null を返し続ける', async () => {
    const previewUrl = `/media/recordings/${crypto.randomUUID()}/preview`;
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(new Response('', { status: 404 }));
    vi.stubGlobal('fetch', fetchMock);

    expect(await loadManifest(previewUrl)).toBeNull();
    expect(await loadManifest(previewUrl)).toBeNull();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  test('中身の壊れた preview.json は、取り直さずに null を返し続ける', async () => {
    const previewUrl = `/media/recordings/${crypto.randomUUID()}/preview`;
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockImplementation(() => Promise.resolve(new Response('{', { status: 200 })));
    vi.stubGlobal('fetch', fetchMock);

    expect(await loadManifest(previewUrl)).toBeNull();
    expect(await loadManifest(previewUrl)).toBeNull();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  test('読めた preview.json は、取り直さない', async () => {
    const previewUrl = `/media/recordings/${crypto.randomUUID()}/preview`;
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockImplementation(() => Promise.resolve(Response.json(manifest)));
    vi.stubGlobal('fetch', fetchMock);

    await loadManifest(previewUrl);
    expect(await loadManifest(previewUrl)).toEqual(manifest);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
