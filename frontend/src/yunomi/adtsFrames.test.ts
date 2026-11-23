import { describe, expect, test } from 'vitest';

import { AdtsSplitter } from './adtsFrames';

// adts は、48kHz、2ch、payload の長さの中身を持つ ADTS のフレームを作る。blocks は raw_data_block の数。
function adts(payload: number, fill: number, blocks = 1): number[] {
  const length = 7 + payload;
  // profile AAC LC (1)、sampling_frequency_index 3 (48kHz)、channel_configuration 2。
  return [
    0xff,
    0xf1,
    (1 << 6) | (3 << 2) | 0,
    (2 << 6) | ((length >> 11) & 0x03),
    (length >> 3) & 0xff,
    ((length & 0x07) << 5) | 0x1f,
    0xfc | ((blocks - 1) & 0x03),
    ...Array.from({ length: payload }, () => fill),
  ];
}

// 48kHz の 1024 サンプルは、PTS で 1920。
const frameTicks = 1920;

describe('AdtsSplitter', () => {
  test('PES の中のフレームを分け、PES の PTS から順に時刻を付ける', () => {
    const splitter = new AdtsSplitter();
    const frames = splitter.push(new Uint8Array([...adts(10, 1), ...adts(20, 2)]), 90_000);

    expect(
      frames.map(({ pts, duration, sampleRate, channels, data }) => ({
        pts,
        duration,
        sampleRate,
        channels,
        length: data.length,
        fill: data[7],
      })),
    ).toEqual([
      { pts: 90_000, duration: frameTicks, sampleRate: 48_000, channels: 2, length: 17, fill: 1 },
      {
        pts: 90_000 + frameTicks,
        duration: frameTicks,
        sampleRate: 48_000,
        channels: 2,
        length: 27,
        fill: 2,
      },
    ]);
  });

  test('PES の境目で切れたフレームは、前の時刻のまま次の PES とつなぎ、次の PES の最初のフレームには、その PTS を当てる', () => {
    const splitter = new AdtsSplitter();
    const first = adts(30, 1);
    const second = adts(10, 2);
    const third = adts(10, 3);

    const a = splitter.push(new Uint8Array(first.slice(0, 12)), 0);
    const b = splitter.push(new Uint8Array([...first.slice(12), ...second, ...third]), 5_000);

    expect(a).toEqual([]);
    expect(b.map(({ pts, data }) => ({ pts, fill: data[7] }))).toEqual([
      { pts: 0, fill: 1 },
      { pts: 5_000, fill: 2 },
      { pts: 5_000 + frameTicks, fill: 3 },
    ]);
  });

  test('PES の中で始まったフレームが次の PES へまたがっても、そのフレームに PES の PTS を当てる', () => {
    const splitter = new AdtsSplitter();
    const first = adts(30, 1);
    const second = adts(30, 2);

    splitter.push(new Uint8Array(first.slice(0, 12)), 0);
    // 2 つ目の PES の PTS は、その中で始まる 2 つ目のフレームの時刻。パケットが落ちて時刻が飛んだ場合。
    const a = splitter.push(new Uint8Array([...first.slice(12), ...second.slice(0, 5)]), 50_000);
    const b = splitter.push(new Uint8Array(second.slice(5)), null);

    expect([...a, ...b].map(({ pts, data }) => ({ pts, fill: data[7] }))).toEqual([
      { pts: 0, fill: 1 },
      { pts: 50_000, fill: 2 },
    ]);
  });

  test('PTS のない PES は、前のフレームの続きの時刻にする', () => {
    const splitter = new AdtsSplitter();
    splitter.push(new Uint8Array(adts(10, 1)), 1_000);
    const frames = splitter.push(new Uint8Array(adts(10, 2)), null);

    expect(frames.map(({ pts }) => pts)).toEqual([1_000 + frameTicks]);
  });

  test('raw_data_block が 2 つのフレームは、2 ブロック分の長さにする', () => {
    const splitter = new AdtsSplitter();
    const frames = splitter.push(new Uint8Array(adts(10, 1, 2)), 0);

    expect(frames.map(({ duration }) => duration)).toEqual([frameTicks * 2]);
  });

  test('同期がずれていたら、次の同期語から読み直す', () => {
    const splitter = new AdtsSplitter();
    const frames = splitter.push(new Uint8Array([0x12, 0x34, ...adts(10, 9)]), 0);

    expect(frames.map(({ data }) => data[7])).toEqual([9]);
  });

  test('reset したら、つなぎ待ちのフレームを捨てる', () => {
    const splitter = new AdtsSplitter();
    splitter.push(new Uint8Array(adts(30, 1).slice(0, 12)), 0);
    splitter.reset();
    const frames = splitter.push(new Uint8Array(adts(10, 2)), 9_000);

    expect(frames.map(({ pts, data }) => ({ pts, fill: data[7] }))).toEqual([
      { pts: 9_000, fill: 2 },
    ]);
  });
});
