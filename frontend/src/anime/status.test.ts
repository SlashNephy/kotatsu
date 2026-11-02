import { describe, expect, test } from 'vitest';

import { filtersOf, matchesFilters, parseAnimeSearch, parseLibraryStatus } from './status';

describe('parseLibraryStatus', () => {
  test.each([
    ['WATCHING', 'WATCHING'],
    ['WANNA_WATCH', 'WANNA_WATCH'],
    ['all', 'all'],
    [undefined, 'WATCHING'],
    ['unknown', 'WATCHING'],
  ])('%s は %s にする', (value, want) => {
    expect(parseLibraryStatus(value)).toBe(want);
  });
});

describe('parseAnimeSearch', () => {
  test('既定の値と知らない値は載せない', () => {
    expect(
      parseAnimeSearch({
        status: 'WATCHING',
        season: '2026-FALL',
        rerun: 'yes',
        period: 'soon',
        weekday: '7',
      }),
    ).toEqual({});
  });

  test('絞り込みを読む', () => {
    expect(
      parseAnimeSearch({
        status: 'all',
        season: '2026-AUTUMN',
        rerun: 'exclude',
        period: 'today',
        weekday: 0,
      }),
    ).toEqual({
      status: 'all',
      season: '2026-AUTUMN',
      rerun: 'exclude',
      period: 'today',
      weekday: 0,
    });
  });
});

describe('matchesFilters', () => {
  // 今は日本時間の 2026 年 11 月 2 日 (月) 20:00。今期は 2026 年の秋。
  const now = Date.UTC(2026, 10, 2, 11);
  const work = {
    status: 'WATCHING',
    seasonYear: 2026,
    season: 'AUTUMN',
    progress: 4,
    // 次の放送は、日本時間の 11 月 4 日 (水) 23:00。
    broadcasts: [
      { countEnd: 5, rerun: false, last: false, program: { startAt: '2026-11-04T14:00:00Z' } },
    ],
    recentBroadcasts: [],
  };
  const filters = filtersOf({});

  test.each<[string, Partial<typeof filters>, boolean]>([
    ['絞り込まなければ当たる', {}, true],
    ['見たいの作品だけ', { status: 'WANNA_WATCH' }, false],
    ['今期', { season: 'current' }, true],
    ['ほかの時期', { season: '2026-SUMMER' }, false],
    ['それより後の放送', { period: 'later' }, true],
    ['今日の放送', { period: 'today' }, false],
    ['水曜', { weekday: 3 }, true],
    ['木曜', { weekday: 4 }, false],
  ])('%s', (_, changes, want) => {
    expect(matchesFilters(work, { ...filters, ...changes }, now)).toBe(want);
  });
});
