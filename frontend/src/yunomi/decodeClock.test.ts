import { describe, expect, test } from 'vitest';

import { DecodeClock } from './decodeClock';

describe('DecodeClock', () => {
  test('組が届くまでは時刻を出さず、届いたら performance の時刻から PTS を出す', () => {
    const clock = new DecodeClock();

    expect(clock.at(100)).toBeNull();
    clock.observe(90_000, 1_000);
    expect(clock.at(1_500)).toBe(90_000 + 500 * 90);
  });

  test('少しの揺れは均し、大きくずれたら合わせ直す', () => {
    const clock = new DecodeClock({ resync: 1_800, blend: 0.1 });
    clock.observe(90_000, 1_000);
    // 10ms (900) 進んだ組が届いた。1 割だけ寄せる。
    clock.observe(90_000 + 900, 1_000);
    expect(clock.at(1_000)).toBeCloseTo(90_090);
    expect(clock.resyncs).toBe(0);

    // 1 秒飛んだ。合わせ直す。
    clock.observe(180_000, 1_000);
    expect(clock.at(1_000)).toBe(180_000);
    expect(clock.resyncs).toBe(1);
  });

  test('reset したら、次の組まで時刻を出さない', () => {
    const clock = new DecodeClock();
    clock.observe(90_000, 1_000);
    clock.reset();

    expect(clock.at(1_000)).toBeNull();
  });
});
