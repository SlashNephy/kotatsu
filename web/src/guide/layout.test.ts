import { afterEach, describe, expect, test } from 'vitest';

import { defaultTimeZone, setTimeZone } from '../futon/timeZone';

import {
  broadcastDay,
  receivedUntil,
  layoutColumn,
  programBox,
  stackedBoxes,
  timeScale,
  timeToY,
  yToTime,
  visibleRange,
  type ProgramTime,
} from './layout';

const minute = 60_000;
// 2026-10-11T04:00:00+09:00
const dayStart = Date.parse('2026-10-11T04:00:00+09:00');
const window = { from: dayStart, to: dayStart + 24 * 60 * minute };

function program(startMinute: number, endMinute: number, endUndecided = false): ProgramTime {
  return {
    startAt: dayStart + startMinute * minute,
    // 終了時刻が未定の番組は、tenban が開始の 1 ミリ秒後を endAt にして返す。
    endAt: endUndecided ? dayStart + startMinute * minute + 1 : dayStart + endMinute * minute,
    endUndecided,
  };
}

describe('programBox', () => {
  test.each([
    {
      name: '時刻に比例した位置と高さ',
      program: program(60, 90),
      end: 90,
      want: { top: 180, height: 90 },
    },
    {
      name: '時間帯の前から続く番組は、上端で切り詰める',
      program: program(-30, 30),
      end: 30,
      want: { top: 0, height: 90 },
    },
    {
      name: '時間帯の後まで続く番組は、下端で切り詰める',
      program: program(24 * 60 - 10, 24 * 60 + 50),
      end: 24 * 60 + 50,
      want: { top: (24 * 60 - 10) * 3, height: 30 },
    },
    {
      name: '短い番組も、名前が読める高さにする',
      program: program(0, 1),
      end: 1,
      want: { top: 0, height: 19 },
    },
  ])('$name', ({ program, end, want }) => {
    expect(programBox(window, program, dayStart + end * minute)).toEqual(want);
  });
});

describe('layoutColumn', () => {
  const at = (m: number) => dayStart + m * minute;

  test.each([
    {
      name: '重ならない番組は、1 つずつ縦に並べる',
      programs: [program(0, 30), program(30, 60)],
      want: { ends: [at(30), at(60)], lanes: [0, 0], laneCounts: [1, 1] },
    },
    {
      name: '終了時刻が未定の番組は、次の番組の開始まで伸ばす',
      programs: [program(0, 0, true), program(90, 120)],
      want: { ends: [at(90), at(120)], lanes: [0, 0], laneCounts: [1, 1] },
    },
    {
      name: '終了時刻が未定で、次の番組がなければ、60 分にする',
      programs: [program(0, 0, true)],
      want: { ends: [at(60)], lanes: [0], laneCounts: [1] },
    },
    {
      // スポーツ中継と、中止のときの番組が、同じ時刻に重なって届く。
      name: '重なる番組は、横に並べる',
      programs: [
        program(0, 10),
        program(10, 230),
        program(10, 70),
        program(70, 75),
        program(230, 260),
      ],
      want: {
        ends: [at(10), at(230), at(70), at(75), at(260)],
        lanes: [0, 0, 1, 1, 0],
        laneCounts: [1, 2, 2, 2, 1],
      },
    },
    {
      name: '終了時刻が未定の番組は、同じ時刻に始まる番組を飛ばして、次に始まる番組まで伸ばす',
      programs: [program(0, 0, true), program(0, 50), program(50, 60)],
      want: { ends: [at(50), at(50), at(60)], lanes: [0, 1, 0], laneCounts: [2, 2, 1] },
    },
  ])('$name', ({ programs, want }) => {
    const layout = layoutColumn(programs);
    expect({ ends: layout.ends, lanes: layout.lanes, laneCounts: layout.laneCounts }).toEqual(want);
  });
});

describe('stackedBoxes', () => {
  test.each([
    {
      // 1 分の番組 (177〜180px) は、終わりを保って 19px に上へ伸ばし、前の番組の終わりを削る。
      name: '短い番組は上へ伸ばし、前の番組の終わりを削る',
      programs: [program(0, 59), program(59, 60), program(60, 120)],
      want: [
        { top: 0, height: 161 },
        { top: 161, height: 19 },
        { top: 180, height: 180 },
      ],
    },
    {
      // 17:55 の 2 分、17:57 の 3 分のように、正時の前に短い番組が続くところ。正時 (180px) の線をまたがない。
      name: '正時の前に短い番組が続いても、正時より前に収める',
      programs: [program(0, 55), program(55, 57), program(57, 60), program(60, 70)],
      want: [
        { top: 0, height: 142 },
        { top: 142, height: 19 },
        { top: 161, height: 19 },
        { top: 180, height: 30 },
      ],
    },
    {
      name: '番組表の上端では上へ伸ばせないので、後ろの番組を下げる',
      programs: [program(0, 1), program(1, 30)],
      want: [
        { top: 0, height: 19 },
        { top: 19, height: 71 },
      ],
    },
    {
      name: '最小の高さより長い番組は、そのまま',
      programs: [program(0, 30), program(30, 60)],
      want: [
        { top: 0, height: 90 },
        { top: 90, height: 90 },
      ],
    },
  ])('$name', ({ programs, want }) => {
    expect(stackedBoxes(window, programs, layoutColumn(programs))).toEqual(want);
  });
});

describe('timeScale', () => {
  test('短い番組がかかる分だけ高くし、ほかの時間は一定の高さのままにする', () => {
    // 59 分からの 1 分の番組。その 1 分だけを、最小の高さ (19px) にする。
    const programs = [program(0, 59), program(59, 60), program(60, 120)];
    const scale = timeScale(window, [{ programs, layout: layoutColumn(programs) }]);

    expect(timeToY(scale, dayStart + 59 * minute)).toBe(177);
    expect(timeToY(scale, dayStart + 60 * minute)).toBe(196);
    // 1 時間後の正時は、伸ばした 16px の分だけ下がる。
    expect(timeToY(scale, dayStart + 120 * minute)).toBe(376);
    expect(yToTime(scale, 196)).toBe(dayStart + 60 * minute);
  });

  test('短い番組では、どの番組も最小の高さに届き、正時に始まる番組は正時の線から始まる', () => {
    // 正時の前の 2 分、3 分の番組と、正時に始まる 5 分の番組。
    const programs = [
      program(0, 55),
      program(55, 57),
      program(57, 60),
      program(60, 65),
      program(65, 120),
    ];
    const layout = layoutColumn(programs);
    const scale = timeScale(window, [{ programs, layout }]);
    const boxes = stackedBoxes(scale, programs, layout);

    expect(boxes.slice(1, 4).every((box) => box.height >= 19)).toBe(true);
    expect(boxes[3]!.top).toBe(timeToY(scale, dayStart + 60 * minute));
  });

  test('目盛りは列をまたいで共通にする', () => {
    const short = [program(0, 1), program(1, 60)];
    const long = [program(0, 60)];
    const scale = timeScale(window, [
      { programs: short, layout: layoutColumn(short) },
      { programs: long, layout: layoutColumn(long) },
    ]);

    expect(stackedBoxes(scale, long, layoutColumn(long))[0]!.height).toBe(
      timeToY(scale, dayStart + 60 * minute),
    );
  });
});

describe('visibleRange', () => {
  // 0〜30、30〜60、60〜(未定、次は 120)、120〜180、180〜240 分
  const programs = [
    program(0, 30),
    program(30, 60),
    program(60, 0, true),
    program(120, 180),
    program(180, 240),
  ];
  const at = (m: number) => dayStart + m * minute;

  const layout = layoutColumn(programs);

  test.each([
    { name: '先頭の番組だけ', from: at(0), to: at(10), want: { start: 0, end: 1 } },
    {
      name: '境界にちょうど接する番組は含めない',
      from: at(30),
      to: at(60),
      want: { start: 1, end: 2 },
    },
    {
      name: '終了時刻が未定の番組は、次の番組の開始まで見える',
      from: at(100),
      to: at(110),
      want: { start: 2, end: 3 },
    },
    { name: '複数にまたがる', from: at(50), to: at(130), want: { start: 1, end: 4 } },
    { name: 'すべての番組の後', from: at(300), to: at(400), want: { start: 5, end: 5 } },
    { name: 'すべての番組の前', from: at(-100), to: at(-50), want: { start: 0, end: 0 } },
  ])('$name', ({ from, to, want }) => {
    expect(visibleRange(programs, layout, from, to)).toEqual(want);
  });

  test('番組がない列', () => {
    expect(visibleRange([], layoutColumn([]), at(0), at(60))).toEqual({ start: 0, end: 0 });
  });

  test('長い番組と重なる短い番組の後ろも、長い番組が見えている間は含める', () => {
    // 0〜240 分の中継と、0〜60 分、60〜120 分の番組が重なる。
    const overlapping = [program(0, 240), program(0, 60), program(60, 120), program(240, 300)];
    expect(visibleRange(overlapping, layoutColumn(overlapping), at(150), at(160))).toEqual({
      start: 0,
      end: 3,
    });
  });
});

describe('broadcastDay', () => {
  test.each([
    {
      name: '4 時より後は、その日の放送日',
      time: '2026-10-11T13:50:00+09:00',
      want: '2026-10-11T04:00:00+09:00',
    },
    {
      name: '4 時より前は、前の日の放送日',
      time: '2026-10-11T03:59:59+09:00',
      want: '2026-10-10T04:00:00+09:00',
    },
    { name: 'ちょうど 4 時', time: '2026-10-11T04:00:00+09:00', want: '2026-10-11T04:00:00+09:00' },
  ])('$name', ({ time, want }) => {
    const day = broadcastDay(Date.parse(time));
    expect(day.from).toBe(Date.parse(want));
    expect(day.to - day.from).toBe(24 * 60 * minute);
  });
});

describe('broadcastDay in other time zones', () => {
  afterEach(() => {
    setTimeZone(defaultTimeZone);
  });

  test.each([
    {
      name: 'ロンドンでは、ロンドンの 4 時から始まる',
      zone: 'Europe/London',
      time: '2026-10-11T13:50:00+01:00',
      want: { from: '2026-10-11T04:00:00+01:00', to: '2026-10-12T04:00:00+01:00' },
    },
    {
      // 2026-10-25 の 2 時に夏時間が終わるので、その日は 25 時間ある。
      name: '夏時間が終わる日は、25 時間ある',
      zone: 'Europe/London',
      time: '2026-10-24T20:00:00+01:00',
      want: { from: '2026-10-24T04:00:00+01:00', to: '2026-10-25T04:00:00+00:00' },
    },
  ])('$name', ({ zone, time, want }) => {
    setTimeZone(zone);

    const day = broadcastDay(Date.parse(time));

    expect({ from: day.from, to: day.to }).toEqual({
      from: Date.parse(want.from),
      to: Date.parse(want.to),
    });
  });
});

describe('receivedUntil', () => {
  const to = Date.parse('2026-10-21T04:00:00+09:00');
  const hour = 3_600_000;

  test.each([
    { name: '1 つも届いていなければ、日の全体を届いていないとする', ends: [], want: 0 },
    {
      name: '途中までしか届いていなければ、いちばん遅い終わりを返す',
      ends: [to - 10 * hour, to - 6 * hour],
      want: to - 6 * hour,
    },
    {
      name: '日の終わりの 1 時間前まで届いていれば、すべて届いたとみなす',
      ends: [to - hour],
      want: undefined,
    },
  ])('$name', ({ ends, want }) => {
    expect(receivedUntil(ends, to)).toBe(want);
  });
});
