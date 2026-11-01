import { describe, expect, test } from 'vitest';

import {
  expectedSize,
  hasRoom,
  offlineSupport,
  parseMeta,
  progressRatio,
  type OfflineMeta,
} from './offline';

const meta: OfflineMeta = {
  version: 1,
  id: 'rec-1',
  name: '番組',
  serviceName: '局',
  startAt: '2026-10-31T12:00:00Z',
  endAt: '2026-10-31T12:30:00Z',
  recordedFrom: '2026-10-31T11:59:30Z',
  recordedUntil: '2026-10-31T12:30:10Z',
  gaps: [{ from: '2026-10-31T12:10:00Z', to: '2026-10-31T12:10:05Z' }],
  size: 123,
  jikkyo: true,
  timeZone: 'Asia/Tokyo',
  savedAt: '2026-11-01T00:00:00Z',
};

describe('parseMeta', () => {
  test.each([
    ['書いたとおりの情報', JSON.stringify(meta), meta],
    ['書きかけで切れた JSON', JSON.stringify(meta).slice(0, 20), null],
    ['古い版', JSON.stringify({ ...meta, version: 0 }), null],
    ['大きさのないもの', JSON.stringify({ ...meta, size: undefined }), null],
    ['形の崩れた途切れ', JSON.stringify({ ...meta, gaps: [{ from: 'x' }] }), null],
    ['オブジェクトでないもの', '[]', null],
  ])('%s', (_, text, want) => {
    expect(parseMeta(text)).toEqual(want);
  });
});

describe('expectedSize', () => {
  const files = [
    { kind: 'SEGMENT', size: '1000' },
    { kind: 'SEGMENT', size: 500 },
    { kind: 'THUMBNAIL', size: 9999 },
  ];

  test.each([
    ['Content-Length があれば、それを使う', '1200', 1200],
    ['Content-Length がなければ、セグメントの和を使う', null, 1500],
    ['Content-Length が数でなければ、セグメントの和を使う', 'abc', 1500],
    ['Content-Length が 0 なら、セグメントの和を使う', '0', 1500],
  ])('%s', (_, header, want) => {
    expect(expectedSize(header, files)).toBe(want);
  });
});

describe('hasRoom', () => {
  test.each([
    ['空きが足りる', { quota: 1000, usage: 400 }, 600, true],
    ['空きが足りない', { quota: 1000, usage: 401 }, 600, false],
    ['使っている量が分からなければ、0 とみなす', { quota: 1000 }, 1000, true],
    ['上限が分からなければ、書いてみる', {}, 10 ** 12, true],
  ])('%s', (_, estimate, required, want) => {
    expect(hasRoom(estimate, required)).toBe(want);
  });
});

describe('offlineSupport', () => {
  class WritableHandle {
    createWritable() {}
  }
  class ReadOnlyHandle {}
  const storage = { getDirectory: () => {}, estimate: () => {} };
  const scope = (handle: unknown, withStorage = true) =>
    ({
      navigator: withStorage ? { storage } : {},
      FileSystemFileHandle: handle,
    }) as unknown as typeof globalThis;

  test.each([
    ['OPFS に書けて、変換できる HD の録画', scope(WritableHandle), true, 'HD', 'supported'],
    ['OPFS に書き込めないブラウザ', scope(ReadOnlyHandle), true, 'HD', 'noStorage'],
    ['OPFS のないブラウザ', scope(WritableHandle, false), true, 'HD', 'noStorage'],
    ['ブラウザの中で変換できない端末', scope(WritableHandle), false, 'HD', 'noConverter'],
    ['BS4K の録画', scope(WritableHandle), true, 'UHD', 'noConverter'],
  ])('%s', (_, target, convert, format, want) => {
    expect(offlineSupport(target, convert, format)).toBe(want);
  });
});

describe('progressRatio', () => {
  test.each([
    [50, 200, 0.25],
    [300, 200, 1],
    [50, null, null],
    [50, 0, null],
  ])('%d / %s', (received, total, want) => {
    expect(progressRatio(received, total)).toBe(want);
  });
});
