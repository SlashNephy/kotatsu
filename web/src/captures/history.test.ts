import { describe, expect, test } from 'vitest';

import { entriesToPrune, matchesQuery } from './history';

describe('entriesToPrune', () => {
  test.each([
    { name: '上限までなら消さない', takenAt: [1, 2, 3], max: 3, want: [] },
    {
      name: '上限を超えた分を、古いものから消す',
      takenAt: [5, 1, 4, 2, 3],
      max: 3,
      want: ['t1', 't2'],
    },
  ])('$name', ({ takenAt, max, want }) => {
    const entries = takenAt.map((at) => ({ id: `t${at}`, takenAt: at }));

    expect(entriesToPrune(entries, max)).toEqual(want);
  });
});

describe('matchesQuery', () => {
  const entry = { serviceName: 'ＮＨＫ総合１・東京', programName: 'ニュース７' };

  test.each([
    { name: '空なら、すべてに合う', query: '  ', want: true },
    { name: '番組名で探す', query: 'ニュース', want: true },
    { name: '全角と半角を区別しない', query: 'nhk', want: true },
    { name: '空白で区切ると、すべてを含むものに合う', query: 'nhk ７', want: true },
    { name: 'どれかを含まなければ合わない', query: 'nhk 天気', want: false },
  ])('$name', ({ query, want }) => {
    expect(matchesQuery(entry, query)).toBe(want);
  });

  test('番組がなくても、サービス名で探せる', () => {
    expect(matchesQuery({ serviceName: 'BS11', programName: null }, 'bs11')).toBe(true);
  });
});
