import { describe, expect, test } from 'vitest';

import {
  BitrateMeter,
  averageBitrate,
  audioFormatKey,
  StreamMeter,
  codecName,
  continuesFrames,
  formatBitrate,
  frameRate,
  nextTsChunk,
  type TsReading,
} from './streamMeter';

describe('BitrateMeter', () => {
  test('直近のかたまりのバイト数を、メディアの秒数で割る', () => {
    const meter = new BitrateMeter();
    meter.add(1_000, 1_000_000, 1);
    meter.add(2_000, 3_000_000, 1);

    expect(meter.rate(2_000)).toBe(16_000_000);
  });

  test('平均する長さより前に届いたかたまりは数えない', () => {
    const meter = new BitrateMeter();
    meter.add(1_000, 10_000_000, 1);
    meter.add(10_000, 1_000_000, 1);

    expect(meter.rate(10_000)).toBe(8_000_000);
  });

  test('かたまりがなければ null', () => {
    const meter = new BitrateMeter();
    meter.add(1_000, 1_000_000, 1);

    expect(meter.rate(60_000)).toBeNull();
  });

  test('reset すると、それまでのかたまりを捨てる', () => {
    const meter = new BitrateMeter();
    meter.add(1_000, 1_000_000, 1);
    meter.reset();

    expect(meter.rate(1_000)).toBeNull();
  });

  test('history は、古い順に 1 秒ごとのビットレートを並べ、かたまりのない秒は null にする', () => {
    const meter = new BitrateMeter();
    meter.add(57_000, 1_000_000, 1);
    meter.add(60_000, 2_000_000, 1);

    const history = meter.history(60_000);

    expect(history).toHaveLength(60);
    expect(history.slice(-5)).toEqual([null, 8_000_000, 8_000_000, 8_000_000, 12_000_000]);
  });
});

describe('nextTsChunk', () => {
  const reading = (load: number, bytesRead: number, mediaEnd: number): TsReading => ({
    load,
    bytesRead,
    mediaEnd,
    seeking: false,
  });

  test.each([
    {
      name: '最初の読みは、区切りにするだけ',
      base: null,
      next: reading(1, 1_000, 1),
      want: { base: reading(1, 1_000, 1), chunk: null },
    },
    {
      name: 'メディアが進んだら、その間に読んだバイト数を返す',
      base: reading(1, 1_000, 1),
      next: reading(1, 3_000, 2),
      want: { base: reading(1, 3_000, 2), chunk: { bytes: 2_000, seconds: 1 } },
    },
    {
      name: 'メディアが進まなければ、区切りを動かさずに溜める',
      base: reading(1, 1_000, 1),
      next: reading(1, 3_000, 1),
      want: { base: reading(1, 1_000, 1), chunk: null },
    },
    {
      name: '読み直したら、新しい区切りにする',
      base: reading(1, 9_000, 9),
      next: reading(2, 1_000, 1),
      want: { base: reading(2, 1_000, 1), chunk: null },
    },
    {
      name: 'シークで読んだ位置が戻ったら、新しい区切りにする',
      base: reading(1, 9_000, 9),
      next: reading(1, 2_000, 12),
      want: { base: reading(1, 2_000, 12), chunk: null },
    },
    {
      name: 'メディアが大きく飛んだら (読み込んだ先へのシーク)、新しい区切りにする',
      base: reading(1, 1_000, 1),
      next: reading(1, 2_000, 30),
      want: { base: reading(1, 2_000, 30), chunk: null },
    },
  ])('$name', ({ base, next, want }) => {
    expect(nextTsChunk(base, next)).toEqual(want);
  });

  test('シークしている間は、区切りを捨てる', () => {
    expect(nextTsChunk(reading(1, 1_000, 1), { ...reading(1, 5_000, 3), seeking: true })).toEqual({
      base: null,
      chunk: null,
    });
  });
});

describe('frameRate', () => {
  test.each([
    {
      name: '再生位置の進みあたりのフレームの数',
      previous: { frames: 0, time: 10 },
      next: { frames: 2997, time: 110 },
      want: 29.97,
    },
    {
      name: '再生位置が進んでいなければ null',
      previous: { frames: 10, time: 10 },
      next: { frames: 10, time: 10 },
      want: null,
    },
    {
      name: 'フレームの数が戻ったら (読み直し) null',
      previous: { frames: 100, time: 10 },
      next: { frames: 3, time: 11 },
      want: null,
    },
  ])('$name', ({ previous, next, want }) => {
    expect(frameRate(previous, next)).toBe(want);
  });
});

describe('formatBitrate', () => {
  test.each([
    { bps: 15_640_000, want: '15.6 Mbps' },
    { bps: 192_000, want: '192 kbps' },
  ])('$want', ({ bps, want }) => {
    expect(formatBitrate(bps)).toBe(want);
  });
});

describe('codecName', () => {
  test.each([
    { codec: 'avc1.64002a', want: 'H.264' },
    { codec: 'hvc1.1.6.L120.90', want: 'HEVC' },
    { codec: 'hev1.1.6.L120.90', want: 'HEVC' },
    { codec: 'mp4a.40.2', want: 'AAC' },
    { codec: 'opus', want: 'opus' },
  ])('$codec', ({ codec, want }) => {
    expect(codecName(codec)).toBe(want);
  });
});

describe('audioFormatKey', () => {
  test.each([
    { componentType: 1, want: 'mono' },
    { componentType: 2, want: 'dualMono' },
    { componentType: 3, want: 'stereo' },
    { componentType: 9, want: 'surround' },
    { componentType: 5, want: null },
  ])('$componentType', ({ componentType, want }) => {
    expect(audioFormatKey(componentType)).toBe(want);
  });
});

describe('averageBitrate', () => {
  const segment = (size: number) => ({ kind: 'SEGMENT', size });
  const now = Date.parse('2026-10-31T00:10:00Z');

  test.each([
    {
      name: 'TS のセグメントの大きさの合計を、録っていた長さで割る',
      recording: {
        recordedFrom: '2026-10-31T00:00:00Z',
        recordedUntil: '2026-10-31T00:01:00Z',
        gaps: [],
        files: [segment(60_000_000), segment(60_000_000)],
      },
      want: 16_000_000,
    },
    {
      name: '途切れた区間は、長さに含めない',
      recording: {
        recordedFrom: '2026-10-31T00:00:00Z',
        recordedUntil: '2026-10-31T00:01:30Z',
        gaps: [{ from: '2026-10-31T00:00:30Z', to: '2026-10-31T00:01:00Z' }],
        files: [segment(120_000_000)],
      },
      want: 16_000_000,
    },
    {
      name: '録画中なら、今までの長さで割る',
      recording: {
        recordedFrom: '2026-10-31T00:09:00Z',
        recordedUntil: null,
        gaps: [],
        files: [segment(120_000_000)],
      },
      want: 16_000_000,
    },
    {
      name: 'TS 以外のファイルは数えない',
      recording: {
        recordedFrom: '2026-10-31T00:00:00Z',
        recordedUntil: '2026-10-31T00:01:00Z',
        gaps: [],
        files: [segment(120_000_000), { kind: 'OTHER', size: 999_999_999 }],
      },
      want: 16_000_000,
    },
    {
      name: 'TS のセグメントがなければ null',
      recording: {
        recordedFrom: '2026-10-31T00:00:00Z',
        recordedUntil: '2026-10-31T00:01:00Z',
        gaps: [],
        files: [{ kind: 'OTHER', size: 999_999_999 }],
      },
      want: null,
    },
  ])('$name', ({ recording, want }) => {
    expect(averageBitrate(recording, now)).toBe(want);
  });
});

describe('continuesFrames', () => {
  test.each([
    {
      name: '経った時間だけ再生位置が進めば、続いている',
      previous: { frames: 0, time: 10 },
      next: { frames: 30, time: 11 },
      rate: 1,
      want: true,
    },
    {
      name: '倍速なら、その分だけ進んでも続いている',
      previous: { frames: 0, time: 10 },
      next: { frames: 60, time: 12 },
      rate: 2,
      want: true,
    },
    {
      name: '先へシークしたら、続いていない',
      previous: { frames: 0, time: 10 },
      next: { frames: 30, time: 21 },
      rate: 1,
      want: false,
    },
    {
      name: '読み直してフレームの数が戻ったら、続いていない',
      previous: { frames: 300, time: 10 },
      next: { frames: 3, time: 11 },
      rate: 1,
      want: false,
    },
  ])('$name', ({ previous, next, rate, want }) => {
    expect(continuesFrames(previous, next, 1, rate)).toBe(want);
  });
});

describe('StreamMeter', () => {
  const reading = (load: number, bytesRead: number, mediaEnd: number) => ({
    load,
    bytesRead,
    mediaEnd,
    seeking: false,
    transport: { dropped: 1, errors: 2, scrambled: 3 },
    source: { width: 1440, height: 1080, interlaced: true },
  });

  test('変換の経路の読みから、ビットレートと TS の異常と放送の映像を溜める', () => {
    const meter = new StreamMeter();
    meter.feedTs(1_000, reading(1, 1_000_000, 1));
    meter.feedTs(2_000, reading(1, 3_000_000, 2));

    expect(meter.bitrate.rate(2_000)).toBe(16_000_000);
    expect(meter.transport).toEqual({ dropped: 1, errors: 2, scrambled: 3 });
    expect(meter.source).toEqual({ width: 1440, height: 1080, interlaced: true });
  });

  test('reset の後の最初の読みは、区切りにするだけ', () => {
    const meter = new StreamMeter();
    meter.feedTs(1_000, reading(1, 1_000_000, 1));
    meter.reset();
    meter.feedTs(2_000, reading(1, 3_000_000, 2));

    expect(meter.bitrate.rate(2_000)).toBeNull();
    expect(meter.transport).toEqual({ dropped: 1, errors: 2, scrambled: 3 });
  });
});
