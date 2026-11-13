import { describe, expect, it } from 'vitest';

import { fetchBundledPackages, filterEntries, type SoftwareEntry } from './licenses';

const entry = (name: string, license: string): SoftwareEntry => ({
  name,
  version: '1.0.0',
  license,
  url: null,
  licenses: [],
  note: null,
});

describe('filterEntries', () => {
  const entries = [
    entry('react', 'MIT'),
    entry('hls.js', 'Apache-2.0'),
    entry('@fontsource/x', 'OFL-1.1'),
  ];

  it.each([
    { name: '空なら全件', query: '  ', want: ['react', 'hls.js', '@fontsource/x'] },
    { name: '名前の一部で、大文字と小文字を区別しない', query: 'HLS', want: ['hls.js'] },
    { name: 'ライセンスで', query: 'ofl', want: ['@fontsource/x'] },
    { name: '一致しない', query: 'gpl', want: [] },
  ])('$name', ({ query, want }) => {
    expect(filterEntries(entries, query).map((e) => e.name)).toEqual(want);
  });
});

describe('fetchBundledPackages', () => {
  it('書き出した一覧を、行の形にし、文書を作ったパッケージには注記を付ける', async () => {
    const fetcher = async () =>
      new Response(
        JSON.stringify([
          {
            name: 'tr46',
            version: '0.0.3',
            license: 'MIT',
            homepage: null,
            files: [{ name: 'LICENSE', text: 'MIT License' }],
            generated: true,
          },
        ]),
        { headers: { 'Content-Type': 'application/json' } },
      );

    const got = await fetchBundledPackages('作った', fetcher);

    expect(got).toEqual([
      {
        name: 'tr46',
        version: '0.0.3',
        license: 'MIT',
        url: null,
        licenses: [{ name: 'LICENSE', text: 'MIT License' }],
        note: '作った',
      },
    ]);
  });

  it('一覧を書き出していないビルドで、画面の HTML が返ったら null', async () => {
    const fetcher = async () =>
      new Response('<!doctype html>', { headers: { 'Content-Type': 'text/html' } });

    expect(await fetchBundledPackages('作った', fetcher)).toBeNull();
  });
});
