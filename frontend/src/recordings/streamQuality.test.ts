import { describe, expect, test } from 'vitest';

import { packetBitrate, streamTypeName, totalQuality } from './streamQuality';

describe('packetBitrate', () => {
  test.each([
    {
      name: '188 バイトのパケットの数を、長さで割る',
      packets: 10_000,
      seconds: 1,
      want: 15_040_000,
    },
    { name: '長さが分からなければ null', packets: 10_000, seconds: 0, want: null },
  ])('$name', ({ packets, seconds, want }) => {
    expect(packetBitrate(packets, seconds)).toBe(want);
  });
});

describe('streamTypeName', () => {
  test.each([
    { streamType: 0x02, want: 'MPEG-2' },
    { streamType: 0x1b, want: 'H.264' },
    { streamType: 0x24, want: 'HEVC' },
    { streamType: 0x0f, want: 'AAC' },
    { streamType: 0x11, want: 'AAC (LATM)' },
    { streamType: 0x06, want: '0x06' },
    { streamType: null, want: null },
  ])('$streamType', ({ streamType, want }) => {
    expect(streamTypeName(streamType)).toBe(want);
  });
});

describe('totalQuality', () => {
  test('PID ごとの数を足し合わせる', () => {
    expect(
      totalQuality([
        { packets: 100, drops: 1, errors: 2, scrambled: 3 },
        { packets: 50, drops: 4, errors: 0, scrambled: 1 },
      ]),
    ).toEqual({ packets: 150, drops: 5, errors: 2, scrambled: 4 });
  });
});
