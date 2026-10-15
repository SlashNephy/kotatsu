import { describe, expect, test } from 'vitest';

import i18next from '../i18n';
import { genreLabel, genreOptions } from './genreOptions';

const t = i18next.t;

describe('genreLabel', () => {
  test.each([
    {
      name: '大分類は、大分類の名前だけにする',
      genre: { lv1: 7, lv2: null },
      want: 'アニメ・特撮',
    },
    {
      name: '中分類は、大分類の名前を前に付ける',
      genre: { lv1: 6, lv2: 2 },
      want: '映画／アニメ',
    },
    {
      name: '名前のない中分類は、番号で出す',
      genre: { lv1: 7, lv2: 9 },
      want: 'アニメ・特撮（中分類9）',
    },
  ])('$name', ({ genre, want }) => {
    expect(genreLabel(t, genre)).toBe(want);
  });
});

describe('genreOptions', () => {
  const valuesOf = (groups: ReturnType<typeof genreOptions>, group: string) =>
    groups.find((g) => g.group === group)?.items.map((item) => item.value);

  test('大分類ごとのグループに、大分類のすべてと中分類を並べる', () => {
    const groups = genreOptions(t, []);

    expect(valuesOf(groups, '映画')).toEqual(['6', '6:0', '6:1', '6:2', '6:15']);
    expect(groups.find((g) => g.group === '映画')?.items[3]?.label).toBe('映画／アニメ');
  });

  test('大分類のその他には、中分類を出さない', () => {
    expect(valuesOf(genreOptions(t, []), 'その他')).toEqual(['15']);
  });

  test('予備と拡張の大分類は出さない', () => {
    const groups = genreOptions(t, []).map((g) => g.group);

    expect(groups).not.toContain('予備');
    expect(groups).not.toContain('拡張');
  });

  test('選んでいる、名前のない中分類も、選択肢に残す', () => {
    const groups = genreOptions(t, [
      { lv1: 7, lv2: 9 },
      { lv1: 14, lv2: 0 },
    ]);

    expect(valuesOf(groups, 'アニメ・特撮')).toEqual(['7', '7:0', '7:1', '7:2', '7:15', '7:9']);
    expect(valuesOf(groups, '拡張')).toEqual(['14:0']);
  });
});
