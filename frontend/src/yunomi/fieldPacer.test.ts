import { describe, expect, test } from 'vitest';

import { FieldPacer } from './fieldPacer';

// clockAt は、PTS = 時刻 (ミリ秒) × 90 + offset の時計を返す。
const clockAt =
  (offset = 0) =>
  (perf: number) =>
    perf * 90 + offset;

// 60Hz の画面の切り替わりの間隔。
const refresh = 1000 / 60;

describe('FieldPacer', () => {
  test('間隔を覚えるまでは何も返さず、覚えたら次の切り替わりの時刻の PTS を返す', () => {
    const pacer = new FieldPacer();

    expect(pacer.onVsync(0, clockAt())).toBeNull();
    expect(pacer.onVsync(refresh, clockAt())).toBeCloseTo(2 * refresh * 90);
  });

  test('切り替わりの時刻が揺れても、PTS は間隔の分ずつ進める', () => {
    const pacer = new FieldPacer();
    // 先に、揺れのない切り替わりで間隔を保持する。
    for (let i = 0; i < 30; i++) {
      pacer.onVsync(i * refresh, clockAt());
    }
    const jitter = [1.5, -1.2, 0.8, -1.4, 1.1, -0.6, 1.3];
    const pts = jitter.map((noise, i) => pacer.onVsync((30 + i) * refresh + noise, clockAt()));
    const steps = pts
      .slice(1)
      .map((value, i) => value! - pts[i]!)
      .map((step) => step / 90);

    // 揺れ (最大 ±1.5ms) をそのまま映すと、刻みは 13〜19ms になる。均すので、間隔の近くにとどまる。
    for (const step of steps) {
      expect(Math.abs(step - refresh)).toBeLessThan(1);
    }
  });

  test('何回分か抜けた切り替わりでは、間隔を変えず、PTS を回数分進める', () => {
    const pacer = new FieldPacer();
    pacer.onVsync(0, clockAt());
    const before = pacer.onVsync(refresh, clockAt())!;
    const after = pacer.onVsync(3 * refresh, clockAt())!;

    expect(pacer.interval).toBeCloseTo(refresh);
    expect((after - before) / 90).toBeCloseTo(2 * refresh, 0);
  });

  test('時計が大きく飛んだら、PTS を時計に合わせ直す', () => {
    const pacer = new FieldPacer();
    pacer.onVsync(0, clockAt());
    pacer.onVsync(refresh, clockAt());
    const jumped = pacer.onVsync(2 * refresh, clockAt(90_000));

    expect(jumped).toBeCloseTo(3 * refresh * 90 + 90_000);
    expect(pacer.resyncs).toBe(1);
  });

  test('2 回分以上と割れる回が続いたら、間隔を測り直す', () => {
    const pacer = new FieldPacer();
    const fast = 1000 / 120;
    let time = 0;
    pacer.onVsync(time, clockAt());
    for (let i = 0; i < 5; i++) {
      time += fast;
      pacer.onVsync(time, clockAt());
    }
    // 画面が 60Hz に下がった。
    for (let i = 0; i < 8; i++) {
      time += refresh;
      pacer.onVsync(time, clockAt());
    }

    expect(pacer.interval).toBeCloseTo(refresh);
  });

  test('裏に回って大きく空いたら、間隔を覚えずに位置だけ覚え直す', () => {
    const pacer = new FieldPacer();
    pacer.onVsync(0, clockAt());
    pacer.onVsync(refresh, clockAt());
    const resumed = pacer.onVsync(5_000, clockAt());

    expect(pacer.interval).toBeCloseTo(refresh);
    expect(resumed).toBeCloseTo((5_000 + refresh) * 90);
    expect(pacer.resyncs).toBe(0);
  });
});
