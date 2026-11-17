import { describe, expect, test } from 'vitest';

import { dramaFiltersOf, dramaPeriodOf, matchesDramaFilters, parseDramaSearch } from './library';

describe('parseDramaSearch', () => {
  test('既定の値と知らない値、アニメの画面だけの絞り込みは載せない', () => {
    expect(
      parseDramaSearch({
        status: 'WATCHING',
        season: '2026-AUTUMN',
        rerun: 'exclude',
        period: 'soon',
        weekday: '7',
      }),
    ).toEqual({});
  });

  test('絞り込みを読む', () => {
    expect(parseDramaSearch({ status: 'WANNA_WATCH', period: 'today', weekday: '3' })).toEqual({
      status: 'WANNA_WATCH',
      period: 'today',
      weekday: 3,
    });
  });
});

describe('dramaPeriodOf と matchesDramaFilters', () => {
  // 今は日本時間の 2026 年 11 月 2 日 (月) 20:00。
  const now = Date.UTC(2026, 10, 2, 11);
  const broadcast = (startAt: string) => ({ rerun: false, last: false, program: { startAt } });

  test.each([
    {
      name: '今日の放送は「今日」',
      work: {
        status: 'WATCHING',
        nextBroadcast: broadcast('2026-11-02T13:00:00Z'),
        recentBroadcast: null,
      },
      period: 'today',
      weekday: 1,
    },
    {
      name: '明日の放送は「明日」',
      work: {
        status: 'WATCHING',
        nextBroadcast: broadcast('2026-11-03T13:00:00Z'),
        recentBroadcast: null,
      },
      period: 'tomorrow',
      weekday: 2,
    },
    {
      name: 'シーズンの最後の話が放送され、これからの放送がなければ「終了」',
      work: {
        status: 'WATCHING',
        nextBroadcast: null,
        recentBroadcast: { startAt: '2026-10-29T13:00:00Z', last: true },
      },
      period: 'ended',
      weekday: 4,
    },
    {
      name: '放送が分からなければ「未定」',
      work: { status: 'WANNA_WATCH', nextBroadcast: null, recentBroadcast: null },
      period: 'unknown',
      weekday: undefined,
    },
  ] as const)('$name', ({ work, period, weekday }) => {
    expect(dramaPeriodOf(work, now)).toBe(period);
    expect(matchesDramaFilters(work, dramaFiltersOf({ status: 'all', period }), now)).toBe(true);
    if (weekday !== undefined) {
      expect(matchesDramaFilters(work, dramaFiltersOf({ status: 'all', weekday }), now)).toBe(true);
      expect(
        matchesDramaFilters(
          work,
          dramaFiltersOf({ status: 'all', weekday: (weekday + 1) % 7 }),
          now,
        ),
      ).toBe(false);
    }
  });

  test('状態の既定は「見ている」', () => {
    const wanna = { status: 'WANNA_WATCH', nextBroadcast: null, recentBroadcast: null };

    expect(matchesDramaFilters(wanna, dramaFiltersOf({}), now)).toBe(false);
    expect(matchesDramaFilters(wanna, dramaFiltersOf({ status: 'WANNA_WATCH' }), now)).toBe(true);
  });
});
