import type { TFunction } from 'i18next';
import { describe, expect, test } from 'vitest';

import ja from '../i18n/ja.json';
import { jobKindLabel } from './jobs';

const t = ((key: string) => key) as unknown as TFunction;

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
