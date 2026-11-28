import { describe, expect, test } from 'vitest';

import {
  createVideoLagController,
  defaultVideoLagParams as params,
  lagMeasurable,
  lagOf,
  type VideoLagController,
} from './videoLag';

// started は、最初の 1 フレーム (読み込み直後で計測しない) を済ませた制御を作る。
function started(measurable = true): VideoLagController {
  const controller = createVideoLagController({ measurable });
  controller.frame(0, { lag: 0, ahead: 1 });

  return controller;
}

describe('飛び直し (測った遅れ)', () => {
  test.each([
    { name: '飛び直す遅れちょうどでは飛ばない', lag: params.lagThreshold, ahead: 1, want: false },
    { name: '飛び直す遅れを超えたら飛ぶ', lag: params.lagThreshold + 0.01, ahead: 1, want: true },
    {
      name: '飛び先の先に読み込んだ量が足りなければ飛ばない',
      lag: 1,
      ahead: params.jumpWidth + params.jumpHeadroom - 0.01,
      want: false,
    },
    {
      name: '飛び先の先に読み込んだ量があれば飛ぶ',
      lag: 1,
      ahead: params.jumpWidth + params.jumpHeadroom,
      want: true,
    },
  ])('$name', ({ lag, ahead, want }) => {
    expect(started().frame(1_000, { lag, ahead })).toBe(want);
  });

  test('読み込み直後の最初のフレームは測らない', () => {
    const controller = createVideoLagController({ measurable: true });

    expect(controller.frame(0, { lag: 1, ahead: 1 })).toBe(false);
    expect(controller.frame(10, { lag: 1, ahead: 1 })).toBe(true);
  });

  test.each([
    {
      name: '飛んだ直後の 1 フレームは測らない',
      prepare: (c: VideoLagController) => c.jumped(0, 'lag', true),
    },
    {
      name: 'seeking を受けた直後の 1 フレームは測らない',
      prepare: (c: VideoLagController) => c.seeking(),
    },
  ])('$name', ({ prepare }) => {
    const controller = started();
    prepare(controller);
    const at = params.minJumpIntervalMs;

    // 飛ぶ前の時刻を持ったフレームは、飛んだ幅の分だけ大きく遅れて見える。
    expect(controller.frame(at, { lag: 2.5, ahead: 1 })).toBe(false);
    expect(controller.frame(at + 33, { lag: 0.5, ahead: 1 })).toBe(true);
  });

  test.each([
    { name: '最短間隔より前は飛ばない', after: params.minJumpIntervalMs - 1, want: false },
    { name: '最短間隔がたてば飛ぶ', after: params.minJumpIntervalMs, want: true },
  ])('$name', ({ after, want }) => {
    const controller = started();
    controller.jumped(1_000, 'lag', true);
    controller.frame(1_010, { lag: 0, ahead: 1 });

    expect(controller.frame(1_000 + after, { lag: 1, ahead: 1 })).toBe(want);
  });

  test('遅れを測れない端末では、測った遅れで飛ばない', () => {
    expect(started(false).frame(1_000, { lag: 1, ahead: 1 })).toBe(false);
  });
});

describe('飛び直し (遅れを測れない端末の詰まり)', () => {
  test.each([
    {
      name: '測れない端末は、詰まってから動き出したら 1 回飛ぶ',
      measurable: false,
      stalls: true,
      want: [true, false],
    },
    {
      name: '測れる端末は、詰まりで飛ばない',
      measurable: true,
      stalls: true,
      want: [false, false],
    },
    { name: '詰まっていなければ飛ばない', measurable: false, stalls: false, want: [false, false] },
  ])('$name', ({ measurable, stalls, want }) => {
    const controller = started(measurable);
    if (stalls) {
      controller.stall();
    }

    // 1 回の詰まりにつき 1 回だけ飛ぶ。
    expect([controller.resumed(1_000, 1), controller.resumed(1_100, 1)]).toEqual(want);
  });

  test('詰まりのたびの飛び直しにも、最短間隔を掛ける', () => {
    const controller = started(false);
    controller.stall();
    expect(controller.resumed(1_000, 1)).toBe(true);
    controller.jumped(1_000, 'stall', false);
    controller.stall();

    expect(controller.resumed(1_000 + params.minJumpIntervalMs - 1, 1)).toBe(false);
  });
});

describe('重さの判定', () => {
  // jumpEvery は、interval ごとに count 回、計測した遅れで飛んだことにする。
  function jumpEvery(
    controller: VideoLagController,
    count: number,
    interval: number,
    counted = true,
  ) {
    for (let i = 0; i < count; i++) {
      controller.jumped(i * interval, 'lag', counted);
    }
  }

  test.each([
    {
      name: '窓の中の飛び直しが回数に満たなければ重くない',
      count: params.heavyJumps - 1,
      interval: 5_000,
      want: false,
    },
    {
      name: '窓の中に回数だけ飛び直したら重い',
      count: params.heavyJumps,
      interval: 5_000,
      want: true,
    },
    {
      name: '窓より間を空けた飛び直しは数えない',
      count: params.heavyJumps,
      interval: params.jumpWindowMs / (params.heavyJumps - 1),
      want: false,
    },
  ])('$name', ({ count, interval, want }) => {
    const controller = started();
    jumpEvery(controller, count, interval);

    expect(controller.heavy((count - 1) * interval)).toBe(want);
  });

  test.each([
    { name: '倍速や早送りの間の飛び直しは数えない', cause: 'lag' as const, counted: false },
    {
      name: '測れない端末の詰まりのたびの飛び直しは数えない',
      cause: 'stall' as const,
      counted: true,
    },
  ])('$name', ({ cause, counted }) => {
    const controller = started();
    for (let i = 0; i < params.heavyJumps; i++) {
      controller.jumped(i * 5_000, cause, counted);
    }

    expect(controller.heavy(10_000)).toBe(false);
  });

  // feed は、1 秒おきに 30 フレームを出し、そのうち ratio の割合を破棄したことにする。
  function feed(controller: VideoLagController, untilMs: number, ratio: number) {
    for (let at = 0; at <= untilMs; at += 1_000) {
      const total = (at / 1_000) * 30;
      controller.quality(at, total, Math.round(total * ratio));
    }
  }

  test.each([
    {
      name: '捨てた割合が閾値ちょうどなら重くない',
      ratio: params.heavyDropRatio,
      until: 20_000,
      want: false,
    },
    {
      name: '捨てた割合が閾値を超えたら重い',
      ratio: params.heavyDropRatio + 0.1,
      until: 20_000,
      want: true,
    },
    // 30 fps なので、最低フレーム数に届くのは 5 秒。
    { name: '最低フレーム数に満たない窓は数えない', ratio: 1, until: 4_000, want: false },
  ])('$name', ({ ratio, until, want }) => {
    const controller = started();
    feed(controller, until, ratio);

    expect(controller.heavy(until)).toBe(want);
  });

  test('数えない間があったら、その前後の差を数えない', () => {
    const controller = started();
    controller.quality(0, 0, 0);
    controller.forgetQuality();
    controller.quality(10_000, 300, 300);
    controller.quality(10_100, 303, 300);

    expect(controller.heavy(10_100)).toBe(false);
  });

  test('窓より長く間が空いたら、その前の記録と比べない', () => {
    const controller = started();
    // 空く前は破棄していない。空いた後の 9 秒に 300 フレームを出し、すべて破棄した。
    controller.quality(0, 0, 0);
    controller.quality(30_000, 300, 0);
    controller.quality(39_000, 600, 300);

    expect(controller.heavy(39_000)).toBe(true);
  });

  test('数えたものを捨てたら、重くなくなる', () => {
    const controller = started();
    jumpEvery(controller, params.heavyJumps, 1_000);
    controller.reset();

    expect(controller.heavy(3_000)).toBe(false);
  });
});

describe('速めと詰まりの扱い', () => {
  test.each([
    {
      name: '遅れが飛び直す遅れを超えたら、速めない時間は速めない',
      at: params.noSpeedUpMs - 1,
      want: false,
    },
    { name: '速めない時間がたてば速めてよい', at: params.noSpeedUpMs, want: true },
  ])('$name', ({ at, want }) => {
    const controller = started();
    // 読み込んだ量が足りず飛べなくても、遅れていれば速めない。
    controller.frame(0, { lag: 1, ahead: 0 });

    expect(controller.speedUpAllowed(at)).toBe(want);
  });

  test.each([
    {
      name: '飛び直した直後の詰まりは、自分で起こしたものとみなす',
      at: params.selfStallGraceMs - 1,
      want: true,
    },
    {
      name: '時間がたった詰まりは、回線の詰まりとして数える',
      at: params.selfStallGraceMs,
      want: false,
    },
  ])('$name', ({ at, want }) => {
    const controller = started();
    controller.jumped(0, 'lag', true);

    expect(controller.excusesStall(at)).toBe(want);
  });
});

describe('lagOf', () => {
  test.each([
    { name: '映像が遅れていれば差', currentTime: 10.5, mediaTime: 10.2, want: 0.3 },
    { name: '負なら 0', currentTime: 10, mediaTime: 10.05, want: 0 },
  ])('$name', ({ currentTime, mediaTime, want }) => {
    expect(lagOf(currentTime, mediaTime)).toBeCloseTo(want);
  });
});

describe('lagMeasurable', () => {
  test.each([
    {
      name: 'userAgentData が Windows なら測れない',
      nav: { userAgent: 'Mozilla/5.0 (X11; Linux x86_64)', userAgentData: { platform: 'Windows' } },
      want: false,
    },
    {
      name: 'userAgentData が Windows 以外なら、UA に関わらず測れる',
      nav: { userAgent: 'Mozilla/5.0 (Windows NT 10.0)', userAgentData: { platform: 'macOS' } },
      want: true,
    },
    {
      name: 'userAgentData が無く、UA に Windows があれば測れない',
      nav: { userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) Firefox/130.0' },
      want: false,
    },
    {
      name: 'userAgentData が無く、UA に Windows が無ければ測れる',
      nav: { userAgent: 'Mozilla/5.0 (iPad; CPU OS 18_0 like Mac OS X)' },
      want: true,
    },
  ])('$name', ({ nav, want }) => {
    expect(lagMeasurable(nav)).toBe(want);
  });
});
