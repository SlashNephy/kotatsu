import { describe, expect, test } from 'vitest';

import {
  type BenchmarkReport,
  formatReport,
  judge,
  median,
  type ScenarioResult,
  summarizeRun,
  summarizeScenario,
} from './benchmarkResults';

const scenario = (
  overrides: Partial<ScenarioResult> & Pick<ScenarioResult, 'id'>,
): ScenarioResult => ({
  fps: 60,
  latePerSecond: 0,
  measured: 3,
  film: false,
  fieldRateDropped: false,
  ...overrides,
});

describe('summarizeRun', () => {
  test.each([
    {
      name: '測った間に増えた、間に合わなかった絵と捨てたコマを、秒で割る',
      start: { time: 1_000, late: 2, dropped: 1 },
      end: { time: 6_000, late: 7, dropped: 6 },
      samples: [59, 61],
      want: { fps: 60, latePerSecond: 2, film: false, fieldRateDropped: false },
    },
    {
      name: '読み直しで累計が戻っても、負の数にしない',
      start: { time: 0, late: 5, dropped: 0 },
      end: { time: 1_000, late: 0, dropped: 0 },
      samples: [30],
      want: { fps: 30, latePerSecond: 0, film: false, fieldRateDropped: false },
    },
    {
      name: 'fps を 1 つも受け取らなければ測れなかったものとする',
      start: { time: 0, late: 0, dropped: 0 },
      end: { time: 1_000, late: 0, dropped: 0 },
      samples: [],
      want: null,
    },
    {
      name: '測った時間がなければ測れなかったものとする',
      start: { time: 1_000, late: 0, dropped: 0 },
      end: { time: 1_000, late: 0, dropped: 0 },
      samples: [60],
      want: null,
    },
  ])('$name', ({ start, end, samples, want }) => {
    expect(summarizeRun(start, end, samples, false, false)).toEqual(want);
  });
});

describe('median', () => {
  test.each([
    { name: '空なら null', values: [], want: null },
    { name: '奇数なら真ん中', values: [3, 1, 2], want: 2 },
    { name: '偶数なら真ん中の 2 つの平均', values: [4, 1, 3, 2], want: 2.5 },
  ])('$name', ({ values, want }) => {
    expect(median(values)).toBe(want);
  });
});

describe('summarizeScenario', () => {
  test('測れた回の中央値を出し、測れなかった回は数えない', () => {
    const run = (fps: number, film = false) => ({
      fps,
      latePerSecond: fps / 10,
      film,
      fieldRateDropped: false,
    });

    expect(summarizeScenario('film', [run(20), null, run(24, true), run(22)])).toEqual({
      id: 'film',
      fps: 22,
      latePerSecond: 2.2,
      measured: 3,
      film: true,
      fieldRateDropped: false,
    });
  });

  test('1 回も測れなければ値は null', () => {
    expect(summarizeScenario('frames', [null, null])).toMatchObject({
      fps: null,
      latePerSecond: null,
      measured: 0,
    });
  });
});

describe('judge', () => {
  const frames = scenario({ id: 'frames', fps: 30, latePerSecond: 0.4 });

  test.each([
    {
      name: '補間は、画面の書き換えの 95% 以上を出し、遅れが増えなければなめらか',
      target: scenario({ id: 'framesInterpolated', fps: 60, latePerSecond: 0.4 }),
      refresh: 1000 / 60,
      want: true,
    },
    {
      name: '総当たりの補間が内蔵 GPU で落ちた数 (41.7 fps) は、追いつかない',
      target: scenario({ id: 'framesInterpolated', fps: 41.7, latePerSecond: 6 }),
      refresh: 1000 / 60,
      want: false,
    },
    {
      name: 'コマは出せても、遅れが 0.5 を超えて増えれば追いつかない',
      target: scenario({ id: 'framesInterpolated', fps: 59, latePerSecond: 1.2 }),
      refresh: 1000 / 60,
      want: false,
    },
    {
      name: '120Hz の画面では、120 回近く出さなければ追いつかない',
      target: scenario({ id: 'framesInterpolated', fps: 60, latePerSecond: 0.4 }),
      refresh: 1000 / 120,
      want: false,
    },
    {
      name: '画面の書き換えの間隔を測れなければ決めない',
      target: scenario({ id: 'framesInterpolated', fps: 60, latePerSecond: 0.4 }),
      refresh: null,
      want: null,
    },
    {
      name: '補間なしの経路は決めない',
      target: frames,
      refresh: 1000 / 60,
      want: null,
    },
  ])('$name', ({ target, refresh, want }) => {
    expect(judge(target, frames, refresh)).toBe(want);
  });

  test.each([
    { name: '59.94 の 95% 以上ならなめらか', result: { fps: 59.6 }, want: true },
    { name: '足りなければ追いつかない', result: { fps: 50 }, want: false },
    {
      name: '途中でフィールドごとの表示をやめたら追いつかない',
      result: { fps: 59.6, fieldRateDropped: true },
      want: false,
    },
  ])('フィールドごとの表示: $name', ({ result, want }) => {
    expect(judge(scenario({ id: 'fields', ...result }), undefined, 1000 / 60)).toBe(want);
  });
});

describe('formatReport', () => {
  test('経路ごとの数と判定と注記を、1 行ずつ並べる', () => {
    const report: BenchmarkReport = {
      userAgent: 'TestAgent/1.0',
      renderer: 'Test GPU',
      refreshInterval: 1000 / 60,
      scenarios: [
        scenario({ id: 'fields', fps: 59.6, latePerSecond: 0.4, fieldRateDropped: true }),
        scenario({ id: 'film', fps: 24, latePerSecond: 0.4, film: true, measured: 2 }),
        scenario({ id: 'filmInterpolated', fps: 60, latePerSecond: 0.4, film: true }),
      ],
      estimate: { ms: 8.04, method: 'readback' },
    };

    expect(formatReport(report, (id) => id)).toBe(
      [
        'User agent: TestAgent/1.0',
        'GPU: Test GPU',
        'Refresh interval: 16.7 ms',
        'Interpolation estimate: 8.0 ms (readback)',
        'fields: 59.6 fps, 0.4 late/s (not smooth, field rate dropped)',
        'film: 24.0 fps, 0.4 late/s (film, 2/3 runs)',
        'filmInterpolated: 60.0 fps, 0.4 late/s (smooth, film)',
      ].join('\n'),
    );
  });
});
