import { describe, expect, test } from 'vitest';

import { chooseRoute, hlsURL, qualities, type Capabilities, type Transcoding } from './route';

const pc: Capabilities = { convert: true, hls: true, hevc: true };
// iPhone は MediaSource を持たず、ManagedMediaSource で hls.js を動かす。HEVC は再生できる。
const iphone: Capabilities = { convert: false, hls: true, hevc: true };
const noHevc: Capabilities = { convert: false, hls: true, hevc: false };
const nothing: Capabilities = { convert: false, hls: false, hevc: false };

const presets = [
  { name: '1080p', codec: 'H264' },
  { name: '720p', codec: 'H264' },
  { name: '360p', codec: 'H264' },
  { name: '720p-hevc', codec: 'HEVC' },
];
const withKyusu: Transcoding = { available: true, presets };
const withoutKyusu: Transcoding = { available: false, presets };

describe('qualities', () => {
  test.each([
    {
      name: 'PC で kyusu がある',
      capabilities: pc,
      transcoding: withKyusu,
      want: ['original', '1080p', '720p', '360p', '720p-hevc'],
    },
    { name: 'PC で kyusu がない', capabilities: pc, transcoding: withoutKyusu, want: ['original'] },
    {
      name: 'HEVC を再生できない端末では、HEVC の画質を出さない',
      capabilities: noHevc,
      transcoding: withKyusu,
      want: ['1080p', '720p', '360p'],
    },
    { name: '何も再生できない端末', capabilities: nothing, transcoding: withKyusu, want: [] },
  ])('$name', ({ capabilities, transcoding, want }) => {
    expect(qualities(capabilities, transcoding)).toEqual(want);
  });
});

describe('chooseRoute', () => {
  test.each([
    {
      name: 'PC では、ブラウザの中での変換を選ぶ',
      capabilities: pc,
      transcoding: withKyusu,
      requested: undefined,
      want: { kind: 'convert' },
    },
    {
      name: 'MediaSource を使えない iPhone では、kyusu の既定の画質を選ぶ',
      capabilities: iphone,
      transcoding: withKyusu,
      requested: undefined,
      want: { kind: 'kyusu', preset: '720p' },
    },
    {
      name: '利用者が選んだ画質を使う',
      capabilities: pc,
      transcoding: withKyusu,
      requested: '360p',
      want: { kind: 'kyusu', preset: '360p' },
    },
    {
      name: '選べない画質を求められたら、既定に戻す',
      capabilities: pc,
      transcoding: withoutKyusu,
      requested: '720p',
      want: { kind: 'convert' },
    },
    {
      name: 'kyusu がなく、ブラウザの中で変換もできない',
      capabilities: iphone,
      transcoding: withoutKyusu,
      requested: undefined,
      want: { kind: 'unavailable', reason: 'noTranscoder' },
    },
    {
      name: 'kyusu はあるが、端末が HLS も再生できない',
      capabilities: nothing,
      transcoding: withKyusu,
      requested: undefined,
      want: { kind: 'unavailable', reason: 'noPlayer' },
    },
  ])('$name', ({ capabilities, transcoding, requested, want }) => {
    expect(chooseRoute(capabilities, transcoding, requested)).toEqual(want);
  });
});

describe('hlsURL', () => {
  test.each([
    {
      name: 'TS の URL に、画質ごとのプレイリストのパスを足す',
      streamUrl: '/streams/services/3273601024',
      want: '/streams/services/3273601024/hls/720p-hevc/playlist.m3u8',
    },
    {
      name: 'クエリは、プレイリストのパスの後ろに付け直す',
      streamUrl: '/streams/services/3273601024?token=abc',
      want: '/streams/services/3273601024/hls/720p-hevc/playlist.m3u8?token=abc',
    },
  ])('$name', ({ streamUrl, want }) => {
    expect(hlsURL(streamUrl, '720p-hevc')).toBe(want);
  });
});
