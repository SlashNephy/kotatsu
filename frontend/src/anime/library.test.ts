import { describe, expect, test } from 'vitest';

import {
  currentSeason,
  nextBroadcastOf,
  periodOf,
  sortWorks,
  weekdayOf,
  type LibraryBroadcast,
  type PastBroadcast,
} from './library';

// jst は、日本時間の 2026 年 11 月 day 日 hour:minute の時刻を返す。
const jst = (day: number, hour: number, minute = 0) => Date.UTC(2026, 10, day, hour - 9, minute);
const iso = (time: number) => new Date(time).toISOString();

const broadcast = (
  start: number,
  countEnd: number | null,
  rerun = false,
  last = false,
): LibraryBroadcast => ({
  countEnd,
  rerun,
  last,
  program: { startAt: iso(start) },
});
const past = (start: number, rerun = false, last = false): PastBroadcast => ({
  startAt: iso(start),
  rerun,
  last,
});

describe('currentSeason', () => {
  test.each([
    [Date.UTC(2026, 0, 1, 0), 2026, 'WINTER'],
    [Date.UTC(2026, 3, 1, 0), 2026, 'SPRING'],
    [Date.UTC(2026, 8, 30, 0), 2026, 'SUMMER'],
    // 日本時間では 2027 年 1 月 1 日。
    [Date.UTC(2026, 11, 31, 16), 2027, 'WINTER'],
    [Date.UTC(2026, 9, 1, 0), 2026, 'AUTUMN'],
  ])('%s は %s 年の %s', (now, year, season) => {
    expect(currentSeason(now)).toEqual({ year, season });
  });
});

describe('nextBroadcastOf', () => {
  const work = {
    progress: 3,
    broadcasts: [
      broadcast(jst(2, 10), 3, true),
      broadcast(jst(2, 12), 4, true),
      broadcast(jst(3, 1), 4),
    ],
    recentBroadcasts: [],
  };

  test('見終えた話の放送を飛ばす', () => {
    expect(nextBroadcastOf(work, false)?.program.startAt).toBe(iso(jst(2, 12)));
  });

  test('再放送を除けば、本放送から選ぶ', () => {
    expect(nextBroadcastOf(work, true)?.program.startAt).toBe(iso(jst(3, 1)));
  });
});

describe('periodOf', () => {
  // 今は 11 月 2 日 (月) の 20:00。放送日は 4 時で区切る。
  const now = jst(2, 20);
  test.each<[string, readonly LibraryBroadcast[], readonly PastBroadcast[], boolean, string]>([
    ['今夜の放送', [broadcast(jst(2, 23), 5)], [], false, 'today'],
    ['深夜 3 時の放送は今日の放送日', [broadcast(jst(3, 3), 5)], [], false, 'today'],
    ['今朝始まった放送', [], [past(jst(2, 8))], false, 'today'],
    ['昨日の放送', [broadcast(jst(9, 23), 6)], [past(jst(1, 23))], false, 'yesterday'],
    ['明日の放送', [broadcast(jst(3, 23), 5)], [], false, 'tomorrow'],
    ['最終話が終わった', [], [past(jst(1, 0) - 2 * 86_400_000, false, true)], false, 'ended'],
    ['来週の放送', [broadcast(jst(9, 23), 5)], [], false, 'later'],
    ['放送の予定がない', [], [], false, 'unknown'],
    [
      '今夜の放送が再放送なら、除くと来週',
      [broadcast(jst(2, 23), 5, true), broadcast(jst(9, 23), 5)],
      [],
      true,
      'later',
    ],
  ])('%s', (_, broadcasts, recentBroadcasts, excludeReruns, want) => {
    expect(periodOf({ progress: 4, broadcasts, recentBroadcasts }, now, excludeReruns)).toBe(want);
  });
});

describe('weekdayOf', () => {
  test('深夜の放送は、放送日の曜日にする', () => {
    // 11 月 3 日 (火) 1:00 の放送は、11 月 2 日 (月) の放送日。
    expect(
      weekdayOf(
        { progress: null, broadcasts: [broadcast(jst(3, 1), 1)], recentBroadcasts: [] },
        false,
      ),
    ).toBe(1);
  });

  test('放送の予定がなければ、最も新しい放送の曜日にする', () => {
    expect(
      weekdayOf({ progress: null, broadcasts: [], recentBroadcasts: [past(jst(1, 12))] }, false),
    ).toBe(0);
  });

  test('放送がなければ決めない', () => {
    expect(
      weekdayOf({ progress: null, broadcasts: [], recentBroadcasts: [] }, false),
    ).toBeUndefined();
  });
});

describe('sortWorks', () => {
  const work = (title: string, broadcasts: readonly LibraryBroadcast[]) => ({
    title,
    progress: null,
    broadcasts,
    recentBroadcasts: [],
  });
  const works = [
    work('再放送だけ', [broadcast(jst(2, 21), 1, true)]),
    work('来週', [broadcast(jst(9, 23), 1)]),
    work('明日', [broadcast(jst(3, 23), 1)]),
    work('予定なし', []),
  ];

  test('次の放送の早い順に並べ、放送の予定のない作品を後ろに回す', () => {
    expect(sortWorks(works, false).map((w) => w.title)).toEqual([
      '再放送だけ',
      '明日',
      '来週',
      '予定なし',
    ]);
  });

  test('再放送を除くと、再放送だけの作品は放送の予定のない作品として後ろに回す', () => {
    expect(sortWorks(works, true).map((w) => w.title)).toEqual([
      '明日',
      '来週',
      '予定なし',
      '再放送だけ',
    ]);
  });
});
