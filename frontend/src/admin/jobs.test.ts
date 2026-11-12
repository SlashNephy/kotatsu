import type { TFunction } from 'i18next';
import { describe, expect, test } from 'vitest';

import ja from '../i18n/ja.json';
import { jobKindLabel, progressPercent, remainingLabel } from './jobs';

const t = ((key: string) => key) as unknown as TFunction;
// tWith は、翻訳のキーに、差し込む値を添えて返す。
const tWith = ((key: string, values?: Record<string, number>) =>
  values === undefined ? key : `${key} ${JSON.stringify(values)}`) as unknown as TFunction;

describe('jobKindLabel', () => {
  test('名前を付けた種類は、翻訳のキーにする', () => {
    expect(jobKindLabel(t, 'thumbnail_generate')).toBe('admin.jobs.kinds.thumbnail_generate');
  });

  test('知らない種類は、そのまま返す', () => {
    expect(jobKindLabel(t, 'something_new')).toBe('something_new');
  });

  test('名前を付けた種類は、どれも日本語の名前がある', () => {
    const kinds = ja.admin.jobs.kinds as Record<string, string>;
    for (const kind of Object.keys(kinds)) {
      expect(jobKindLabel(t, kind)).toBe(`admin.jobs.kinds.${kind}`);
    }
    expect(Object.keys(kinds)).toHaveLength(25);
  });
});

describe('progressPercent', () => {
  test.each([
    { ratio: 0, want: 0 },
    { ratio: 0.426, want: 42 },
    // 頭打ちの 99% は、書き終えるまで 100% と出さない。
    { ratio: 0.999, want: 99 },
    { ratio: 1, want: 100 },
  ])('$ratio は $want%', ({ ratio, want }) => {
    expect(progressPercent(ratio)).toBe(want);
  });
});

describe('remainingLabel', () => {
  test.each([
    { seconds: null, want: 'admin.jobs.remainingUnknown' },
    { seconds: 0, want: 'admin.jobs.remainingUnderMinute' },
    { seconds: 59, want: 'admin.jobs.remainingUnderMinute' },
    // 分は切り上げる。
    { seconds: 61, want: 'admin.jobs.remainingMinutes {"minutes":2}' },
    { seconds: 3540, want: 'admin.jobs.remainingMinutes {"minutes":59}' },
    { seconds: 3541, want: 'admin.jobs.remainingHours {"hours":1,"minutes":0}' },
    { seconds: 2 * 3600 + 5 * 60, want: 'admin.jobs.remainingHours {"hours":2,"minutes":5}' },
  ])('$seconds 秒', ({ seconds, want }) => {
    expect(remainingLabel(tWith, seconds)).toBe(want);
  });
});
