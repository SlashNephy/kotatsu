import { describe, expect, test } from 'vitest';

import {
  breadcrumbs,
  clampSeek,
  isMyDrive,
  libraryResumePosition,
  parentPath,
  parseLibrarySearch,
  seekLimit,
  type FileIndex,
} from './library';

const complete: FileIndex = { complete: true, seekableSeconds: 1795, durationSeconds: 1800 };
const indexing: FileIndex = { complete: false, seekableSeconds: 300, durationSeconds: 1800 };
const notYet: FileIndex = { complete: false, seekableSeconds: 0, durationSeconds: null };

describe('parseLibrarySearch', () => {
  test.each([
    {
      name: '両方',
      search: { library: 'anime', path: 'a/b' },
      want: { library: 'anime', path: 'a/b' },
    },
    { name: '空は省く', search: { library: '', path: '' }, want: {} },
    { name: '文字列でないものは捨てる', search: { library: 1, path: ['a'] }, want: {} },
  ])('$name', ({ search, want }) => {
    expect(parseLibrarySearch(search)).toEqual(want);
  });
});

describe('breadcrumbs', () => {
  test.each([
    { path: '', want: [{ name: 'anime', path: '' }] },
    {
      path: '2026/秋',
      want: [
        { name: 'anime', path: '' },
        { name: '2026', path: '2026' },
        { name: '秋', path: '2026/秋' },
      ],
    },
  ])('$path', ({ path, want }) => {
    expect(breadcrumbs('anime', path)).toEqual(want);
  });

  // Google Drive では、段が ID なので、サーバーが返した名前で出す (#1902)。
  test('段ごとの名前', () => {
    expect(breadcrumbs('Google Drive', 'root/f1', ['My Drive', 'アニメ'])).toEqual([
      { name: 'Google Drive', path: '' },
      { name: 'My Drive', path: 'root' },
      { name: 'アニメ', path: 'root/f1' },
    ]);
  });
});

describe('isMyDrive', () => {
  test.each([
    { kind: 'GOOGLE_DRIVE', path: 'root', want: true },
    { kind: 'GOOGLE_DRIVE', path: 'root/f1', want: false },
    { kind: 'FOLDER', path: 'root', want: false },
  ])('$kind $path', ({ kind, path, want }) => {
    expect(isMyDrive(kind, path)).toBe(want);
  });
});

describe('parentPath', () => {
  test.each([
    { path: 'a.ts', want: '' },
    { path: 'show/ep1.ts', want: 'show' },
    { path: 'a/b/c.ts', want: 'a/b' },
  ])('$path', ({ path, want }) => {
    expect(parentPath(path)).toBe(want);
  });
});

describe('seekLimit と clampSeek', () => {
  test.each([
    { name: '作り終えたら長さまで', index: complete, target: 2000, limit: 1800, want: 1800 },
    { name: '作っている途中は、できた所まで', index: indexing, target: 900, limit: 300, want: 300 },
    { name: 'できた所の手前はそのまま', index: indexing, target: 120, limit: 300, want: 120 },
    { name: '索引がまだなければ、頭だけ', index: notYet, target: 60, limit: 0, want: 0 },
    { name: '負は 0', index: complete, target: -5, limit: 1800, want: 0 },
  ])('$name', ({ index, target, limit, want }) => {
    expect(seekLimit(index)).toBe(limit);
    expect(clampSeek(target, index)).toBe(want);
  });
});

describe('libraryResumePosition', () => {
  test.each([
    { name: '保存していなければ頭から', savedMs: null, index: complete, want: 0 },
    { name: '少し戻して続きから', savedMs: 600_000, index: complete, want: 595 },
    { name: '終わりの近くまで見ていたら頭から', savedMs: 1_795_000, index: complete, want: 0 },
    { name: '索引が届いていなければ、届いた所まで', savedMs: 600_000, index: indexing, want: 300 },
    { name: '索引がまだなければ頭から', savedMs: 600_000, index: notYet, want: 0 },
  ])('$name', ({ savedMs, index, want }) => {
    expect(libraryResumePosition(savedMs, index)).toBe(want);
  });
});
