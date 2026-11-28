import { describe, expect, test } from 'vitest';

import { assembleFilm, type FieldFrame, FieldSampler, TelecineDetector } from './telecine';

// diffsOf は、位相 phase の 2:3 プルダウンで、通し番号 from から count 枚のフィールドの差を返す。
// 繰り返すフィールドは repeated、ほかは moving にする。
function diffsOf(phase: number, from: number, count: number, moving = 10, repeated = 0.5) {
  return Array.from({ length: count }, (_, index) => {
    const q = (((from + index - phase) % 10) + 10) % 10;

    return q === 2 || q === 7 ? repeated : moving;
  });
}

function feed(detector: TelecineDetector, from: number, diffs: number[]): number | null {
  let phase: number | null = null;
  diffs.forEach((diff, index) => {
    phase = detector.push(from + index, diff);
  });

  return phase;
}

describe('TelecineDetector', () => {
  test.each([0, 1, 2, 3, 4])('位相 %i の 2:3 プルダウンを見つける', (phase) => {
    const detector = new TelecineDetector();

    expect(feed(detector, 0, diffsOf(phase, 0, 40))).toBe(phase);
  });

  test('位相は 5 通りで、周の後半から数えても同じ位相に決まる', () => {
    const detector = new TelecineDetector();

    expect(feed(detector, 0, diffsOf(7, 0, 40))).toBe(2);
  });

  test('直近の 40 フィールドがそろうまでは決めない', () => {
    const detector = new TelecineDetector();

    expect(feed(detector, 0, diffsOf(1, 0, 39))).toBeNull();
  });

  test('どのフィールドも同じくらい動いていれば (60i)、プルダウンとみなさない', () => {
    const detector = new TelecineDetector();

    expect(
      feed(
        detector,
        0,
        Array.from({ length: 40 }, () => 10),
      ),
    ).toBeNull();
  });

  test('止まった絵では、決めた位相を保つ', () => {
    const detector = new TelecineDetector();
    feed(detector, 0, diffsOf(3, 0, 40));

    expect(
      feed(
        detector,
        40,
        Array.from({ length: 40 }, () => 0.2),
      ),
    ).toBe(3);
  });

  test('繰り返しがなくなれば (60i の CM に替わった)、位相を破棄する', () => {
    const detector = new TelecineDetector();
    feed(detector, 0, diffsOf(3, 0, 40));

    expect(
      feed(
        detector,
        40,
        Array.from({ length: 40 }, () => 10),
      ),
    ).toBeNull();
  });

  test('位相がずれたら (編集の継ぎ目)、新しい位相に決め直す', () => {
    const detector = new TelecineDetector();
    feed(detector, 0, diffsOf(3, 0, 40));

    expect(feed(detector, 40, diffsOf(1, 40, 40))).toBe(1);
  });

  test('通し番号が飛んだら、数え直す', () => {
    const detector = new TelecineDetector();
    feed(detector, 0, diffsOf(3, 0, 40));

    expect(detector.push(50, 10)).toBeNull();
  });
});

describe('assembleFilm', () => {
  const duration = 3003;
  const frame = (index: number, topFieldFirst = true): FieldFrame<string> => ({
    index,
    pts: 90000 + index * duration,
    topFieldFirst,
    source: `f${index}`,
  });

  test('5 フレームから、上と下のフィールドを組み合わせた 4 フレームを、等しい間隔で出す', () => {
    const films = [0, 1, 2, 3, 4].flatMap((index) =>
      assembleFilm(0, frame(index), index === 0 ? null : frame(index - 1), duration),
    );

    expect(films).toEqual([
      { pts: 90000, top: 'f0', bottom: 'f0' },
      { pts: 90000 + 1.25 * duration, top: 'f2', bottom: 'f1' },
      { pts: 90000 + 2.5 * duration, top: 'f3', bottom: 'f2' },
      { pts: 90000 + 3.75 * duration, top: 'f4', bottom: 'f4' },
    ]);
  });

  test('下のフィールドが先のフレームでは、先のフィールドを下として組み合わせる', () => {
    const films = [0, 1, 2, 3, 4].flatMap((index) =>
      assembleFilm(0, frame(index, false), index === 0 ? null : frame(index - 1, false), duration),
    );

    expect(films.map(({ top, bottom }) => [top, bottom])).toEqual([
      ['f0', 'f0'],
      ['f1', 'f2'],
      ['f2', 'f3'],
      ['f4', 'f4'],
    ]);
  });

  test('組み合わせる前のフレームがなければ、そのフレームは出さない', () => {
    expect(assembleFilm(0, frame(2), null, duration)).toEqual([]);
  });
});

describe('FieldSampler', () => {
  // picture は、width × height の輝度の面を返す。上のフィールドの行は top、下のフィールドの行は bottom で埋める。
  function picture(width: number, height: number, top: number, bottom: number): Uint8Array {
    const heap = new Uint8Array(width * height);
    for (let y = 0; y < height; y += 1) {
      heap.fill(y % 2 === 0 ? top : bottom, y * width, (y + 1) * width);
    }

    return heap;
  }

  test('1 つ前のフレームの、同じ向きのフィールドとの差を出す', () => {
    const sampler = new FieldSampler();

    expect(sampler.sample(picture(16, 16, 10, 20), 0, 16, 16, 16)).toBeNull();
    expect(sampler.sample(picture(16, 16, 10, 50), 0, 16, 16, 16)).toEqual({ top: 0, bottom: 30 });
    expect(sampler.sample(picture(16, 16, 40, 50), 0, 16, 16, 16)).toEqual({ top: 30, bottom: 0 });
  });

  test('大きさが変われば比較しない', () => {
    const sampler = new FieldSampler();
    sampler.sample(picture(16, 16, 10, 20), 0, 16, 16, 16);

    expect(sampler.sample(picture(32, 16, 10, 20), 0, 32, 32, 16)).toBeNull();
  });
});
