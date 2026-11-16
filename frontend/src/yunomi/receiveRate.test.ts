import { describe, expect, test } from 'vitest';

import { ReceiveMeter } from './receiveRate';

describe('ReceiveMeter.delivered', () => {
  test.each([
    {
      name: '窓の中に届いた量を窓の長さで割る',
      adds: [
        { at: 0, bytes: 100_000 },
        { at: 1_500, bytes: 375_000 },
        { at: 3_000, bytes: 375_000 },
      ],
      limitedAt: null,
      now: 3_000,
      // 窓 (3 秒) の端の 0 ミリ秒の塊も含める: 850,000 バイト × 8 ÷ 3 秒
      want: 2_266_666.6666666665,
    },
    {
      name: '測り始めてから窓の長さがたっていなければ null',
      adds: [{ at: 1_000, bytes: 100_000 }],
      limitedAt: null,
      now: 3_000,
      want: null,
    },
    {
      name: '窓の外の塊は数えない',
      adds: [
        { at: 0, bytes: 1_000_000 },
        { at: 9_000, bytes: 375_000 },
      ],
      limitedAt: null,
      now: 10_000,
      want: 1_000_000,
    },
    {
      name: '窓の中で読むのを絞っていたら null',
      adds: [
        { at: 0, bytes: 100_000 },
        { at: 5_000, bytes: 100_000 },
      ],
      limitedAt: 4_000,
      now: 5_000,
      want: null,
    },
    {
      name: '絞っていたのが窓より前なら測る',
      adds: [
        { at: 0, bytes: 100_000 },
        { at: 5_000, bytes: 375_000 },
      ],
      limitedAt: 1_000,
      now: 5_000,
      want: 1_000_000,
    },
  ])('$name', ({ adds, limitedAt, now, want }) => {
    const meter = new ReceiveMeter();
    for (const { at, bytes } of adds) {
      meter.add(at, bytes);
    }
    if (limitedAt !== null) {
      meter.limit(limitedAt);
    }
    expect(meter.delivered(now, 3_000)).toBe(want);
  });
});

describe('ReceiveMeter.throughput', () => {
  test.each([
    {
      name: '読み終えた塊の量を、読むのにかかった時間の合計で割る',
      adds: [
        { at: 1_000, bytes: 500_000, transferMs: 200 },
        { at: 3_000, bytes: 500_000, transferMs: 300 },
      ],
      now: 4_000,
      // 1,000,000 バイト × 8 ÷ 0.5 秒
      want: 16_000_000,
    },
    {
      name: '読むのにかかった時間が分からない塊は数えない',
      adds: [
        { at: 1_000, bytes: 500_000, transferMs: null },
        { at: 3_000, bytes: 500_000, transferMs: 500 },
      ],
      now: 4_000,
      want: 8_000_000,
    },
    {
      name: '見る長さより古い塊は数えない',
      adds: [{ at: 1_000, bytes: 500_000, transferMs: 200 }],
      now: 12_000,
      want: null,
    },
  ])('$name', ({ adds, now, want }) => {
    const meter = new ReceiveMeter();
    for (const { at, bytes, transferMs } of adds) {
      meter.add(at, bytes, transferMs);
    }
    expect(meter.throughput(now)).toBe(want);
  });
});

describe('ReceiveMeter.receivedBetween', () => {
  test.each([
    { name: '区間の両端を含めて、届いた量を足す', from: 1_000, to: 3_000, want: 600_000 },
    { name: '区間の外の塊は数えない', from: 1_500, to: 2_500, want: 200_000 },
    { name: '何も届いていなければ 0', from: 4_000, to: 5_000, want: 0 },
  ])('$name', ({ from, to, want }) => {
    const meter = new ReceiveMeter();
    meter.add(1_000, 100_000);
    meter.add(2_000, 200_000);
    meter.add(3_000, 300_000);
    expect(meter.receivedBetween(from, to)).toBe(want);
  });
});
