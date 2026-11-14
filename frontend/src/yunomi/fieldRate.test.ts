import { describe, expect, test } from 'vitest';

import { createLateWatch, lateFieldsPerSecond, lateSeconds, loadFieldRate } from './fieldRate';

describe('loadFieldRate', () => {
  test.each([
    { name: '覚えていなければフィールドごとに表示する', stored: null, want: true },
    { name: '入れたことを覚えていれば表示する', stored: 'true', want: true },
    { name: '切ったことを覚えていれば表示しない', stored: 'false', want: false },
  ])('$name', ({ stored, want }) => {
    expect(loadFieldRate({ getItem: () => stored })).toBe(want);
  });

  test('読めなければフィールドごとに表示する', () => {
    expect(
      loadFieldRate({
        getItem: () => {
          throw new Error('denied');
        },
      }),
    ).toBe(true);
  });
});

// lateSequence は、1 秒ごとに deltas だけ late が増える累計の列を作る。
function lateSequence(deltas: number[]): number[] {
  let late = 0;

  return deltas.map((delta) => (late += delta));
}

describe('createLateWatch', () => {
  const slow = lateFieldsPerSecond;
  const fine = lateFieldsPerSecond - 1;

  test.each([
    {
      name: '追いつかない秒が続けばやめる',
      reports: lateSequence([0, ...Array<number>(lateSeconds).fill(slow)]).map((late) => ({
        late,
        film: false,
      })),
      want: true,
    },
    {
      name: '続く秒が足りなければやめない',
      reports: lateSequence([0, ...Array<number>(lateSeconds - 1).fill(slow)]).map((late) => ({
        late,
        film: false,
      })),
      want: false,
    },
    {
      name: '間に合わない数が閾値より少なければやめない',
      reports: lateSequence([0, ...Array<number>(lateSeconds * 2).fill(fine)]).map((late) => ({
        late,
        film: false,
      })),
      want: false,
    },
    {
      name: '間に追いつく秒を挟めば数え直す',
      reports: lateSequence([
        0,
        ...Array<number>(lateSeconds - 1).fill(slow),
        0,
        ...Array<number>(lateSeconds - 1).fill(slow),
      ]).map((late) => ({ late, film: false })),
      want: false,
    },
    {
      name: '24p で出している間は数えない',
      reports: lateSequence([0, ...Array<number>(lateSeconds).fill(slow)]).map((late) => ({
        late,
        film: true,
      })),
      want: false,
    },
    {
      // 最初の知らせは、それまでの累計なので、1 秒の数としては使わない。
      name: '最初の知らせの累計は数えない',
      reports: [
        { late: slow * 100, film: false },
        ...lateSequence(Array<number>(lateSeconds - 1).fill(slow)).map((late) => ({
          late: slow * 100 + late,
          film: false,
        })),
      ],
      want: false,
    },
    {
      // 読み直すと累計が 0 に戻る。減った知らせを、大きな増えとして数えない。
      name: '累計が戻れば、そこから数え直す',
      reports: [
        ...lateSequence([0, ...Array<number>(lateSeconds - 1).fill(slow)]).map((late) => ({
          late,
          film: false,
        })),
        { late: 0, film: false },
      ],
      want: false,
    },
  ])('$name', ({ reports, want }) => {
    const watch = createLateWatch();
    const results = reports.map((report) => watch.observe(report));
    expect(results.at(-1)).toBe(want);
  });

  test('reset の後は、前の累計を引かずに数え直す', () => {
    const watch = createLateWatch();
    for (const late of lateSequence([0, ...Array<number>(lateSeconds - 1).fill(slow)])) {
      watch.observe({ late, film: false });
    }
    watch.reset();
    expect(watch.observe({ late: slow * 100, film: false })).toBe(false);
  });
});
