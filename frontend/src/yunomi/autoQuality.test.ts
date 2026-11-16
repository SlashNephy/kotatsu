import { beforeEach, describe, expect, test } from 'vitest';

import {
  capRoute,
  decodeStepDown,
  ladder,
  loadDecodeCap,
  lowerDecodeCap,
  resetDecodeCapForTest,
} from './autoQuality';
import { original, type PlaybackRoute, type Preset } from './route';

const presets: Preset[] = [
  { name: '1080p', codec: 'H264', passthrough: false },
  { name: '720p', codec: 'H264', passthrough: false },
  { name: '540p', codec: 'H264', passthrough: false },
  { name: '360p', codec: 'H264', passthrough: false },
  { name: '1080p60', codec: 'H264', passthrough: false },
  { name: '720p-hevc', codec: 'HEVC', passthrough: false },
  { name: 'source', codec: 'HEVC', passthrough: true },
];
const all = [original, ...presets.map((preset) => preset.name)];

describe('ladder', () => {
  test.each([
    {
      name: 'オリジナルと kyusu の H.264 の段を高い順に並べ、60p と HEVC と変換しない画質は載せない',
      choices: all,
      want: [original, '1080p', '720p', '540p', '360p'],
    },
    { name: 'kyusu が無ければオリジナルだけ', choices: [original], want: [original] },
    {
      name: 'オリジナルが選べなければ kyusu の段だけ',
      choices: ['720p', '360p'],
      want: ['720p', '360p'],
    },
  ])('$name', ({ choices, want }) => {
    expect(ladder(choices, presets).map((rung) => rung.name)).toEqual(want);
  });
});

describe('decodeStepDown', () => {
  test.each([
    { name: 'オリジナルからは 720p へ移す', current: original, choices: all, want: '720p' },
    {
      name: 'オリジナルから、720p が無ければ、その下で最も高い段へ移す',
      current: original,
      choices: [original, '1080p', '540p'],
      want: '540p',
    },
    { name: 'kyusu の段は 1 段下げる', current: '1080p', choices: all, want: '720p' },
    { name: '540p から 360p へ', current: '540p', choices: all, want: '360p' },
    { name: '最も低い段からは下げない', current: '360p', choices: all, want: null },
    { name: '60p からは、その高さより下の段へ', current: '1080p60', choices: all, want: '720p' },
    { name: 'HEVC からも、その高さより下の段へ', current: '720p-hevc', choices: all, want: '540p' },
    {
      // BS4K の番組では、オリジナル (ブラウザの中での変換) は選べない。
      name: '変換しない画質 (2160p) からは 1080p へ',
      current: 'source',
      choices: all.filter((name) => name !== original),
      want: '1080p',
    },
    {
      name: 'kyusu が無ければ、オリジナルからは下げない',
      current: original,
      choices: [original],
      want: null,
    },
  ])('$name', ({ current, choices, want }) => {
    expect(decodeStepDown(current, ladder(choices, presets))?.name ?? null).toBe(want);
  });
});

describe('capRoute', () => {
  const convert: PlaybackRoute = { kind: 'convert' };
  test.each([
    { name: '上限が無ければそのまま', base: convert, cap: null, choices: all, want: convert },
    {
      name: 'オリジナルは、上限 720 で 720p へ収める',
      base: convert,
      cap: 720,
      choices: all,
      want: { kind: 'kyusu', preset: '720p' },
    },
    {
      name: '上限以下の経路はそのまま',
      base: { kind: 'kyusu', preset: '540p' } as const,
      cap: 720,
      choices: all,
      want: { kind: 'kyusu', preset: '540p' },
    },
    {
      name: '変換しない画質は、上限 1080 で 1080p へ収める',
      base: { kind: 'kyusu', preset: 'source' } as const,
      cap: 1080,
      choices: all,
      want: { kind: 'kyusu', preset: '1080p' },
    },
    {
      name: '収める段が無ければそのまま',
      base: convert,
      cap: 720,
      choices: [original],
      want: convert,
    },
    {
      name: '再生できない経路はそのまま',
      base: { kind: 'unavailable', reason: 'shv' } as const,
      cap: 360,
      choices: all,
      want: { kind: 'unavailable', reason: 'shv' },
    },
  ])('$name', ({ base, cap, choices, want }) => {
    expect(capRoute(base, cap, ladder(choices, presets))).toEqual(want);
  });
});

describe('デコードの上限', () => {
  beforeEach(() => resetDecodeCapForTest());

  test.each([
    { name: '最初に下げた高さを覚える', steps: [720], want: 720 },
    { name: 'さらに下げたら低い方を覚える', steps: [720, 540], want: 540 },
    { name: '高い方へは戻さない', steps: [540, 720], want: 540 },
  ])('$name', ({ steps, want }) => {
    for (const step of steps) {
      lowerDecodeCap(step);
    }

    expect(loadDecodeCap()).toBe(want);
  });
});
