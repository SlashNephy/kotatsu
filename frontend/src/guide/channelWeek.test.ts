import { describe, expect, test } from 'vitest';

import { channelWeekColumns } from './channelWeek';
import type { GuideProgram } from './GuideGrid';

const hour = 3_600_000;
// today は、2026-10-17 (土) 12:00 (日本時間)。放送日は 2026-10-17 04:00 (日本時間) に始まる。
const today = Date.parse('2026-10-17T03:00:00Z');
const dayStart = Date.parse('2026-10-16T19:00:00Z');

function program(id: string, startAt: number, minutes: number): GuideProgram {
  return {
    id,
    startAt,
    endAt: startAt + minutes * 60_000,
    endUndecided: false,
    name: id,
    genre: undefined,
    shared: false,
    reserved: false,
    watchReserved: false,
  };
}

describe('channelWeekColumns', () => {
  test('放送日ごとの列に分け、各列の番組の時刻を今日の放送日へずらす', () => {
    const programs = [
      // 今日の放送日の前 (3 時半) に始まった番組は、前の放送日のものなので出さない。
      program('昨日の 27 時半', dayStart - hour / 2, 60),
      program('今日の 21 時', dayStart + 17 * hour, 60),
      program('今日の 27 時 (翌日の 3 時)', dayStart + 23 * hour, 30),
      program('明日の 4 時', dayStart + 24 * hour, 60),
      program('7 日後の 12 時', dayStart + (7 * 24 + 8) * hour, 30),
      program('8 日後', dayStart + 8 * 24 * hour, 30),
    ];

    const columns = channelWeekColumns(
      today,
      'S',
      (from) => new Date(from).toISOString(),
      programs,
      8,
    );

    expect(columns.map((column) => column.id)).toEqual([
      'S:0',
      'S:1',
      'S:2',
      'S:3',
      'S:4',
      'S:5',
      'S:6',
      'S:7',
    ]);
    expect(columns[0]?.name).toBe('2026-10-16T19:00:00.000Z');
    expect(columns.map((column) => column.programs.map((p) => p.id))).toEqual([
      ['今日の 21 時', '今日の 27 時 (翌日の 3 時)'],
      ['明日の 4 時'],
      [],
      [],
      [],
      [],
      [],
      ['7 日後の 12 時'],
    ]);
    // 明日の 4 時の番組は、今日の放送日の 4 時の位置に置く。長さは変えない。
    expect(columns[1]?.programs[0]?.startAt).toBe(dayStart);
    expect(columns[1]?.programs[0]?.endAt).toBe(dayStart + hour);
    expect(columns[7]?.programs[0]?.startAt).toBe(dayStart + 8 * hour);
  });

  test('列ごとに、EPG が届いている時刻を、今日の放送日へずらして持つ', () => {
    const programs = [
      // 今日は 4 時から 27 時半まで届いている。日の終わりの 1 時間前までに届いていれば、すべて届いたとみなす。
      program('今日', dayStart, 23.5 * 60),
      // 明日は、今日から続く番組の後、12 時まで届いている。
      program('明日の 3 時半から', dayStart + 23.5 * hour, 30),
      program('明日の 4 時', dayStart + 24 * hour, 8 * 60),
    ];

    const columns = channelWeekColumns(today, 'S', String, programs, 3);

    expect(columns.map((column) => column.receivedUntil)).toEqual([
      undefined,
      dayStart + 8 * hour,
      // 明後日は 1 つも届いていないので、日の始まりから届いていないとする。
      dayStart,
    ]);
  });
});
