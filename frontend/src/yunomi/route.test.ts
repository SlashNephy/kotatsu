import { describe, expect, test } from 'vitest';

import {
  chooseRoute,
  dataBroadcastSource,
  hlsURL,
  qualities,
  toVideoFormat,
  type Capabilities,
  type Transcoding,
} from './route';

// pc は、HEVC の 2160p の 10 ビットを復号できない PC (Linux の Chrome など)。
const pc: Capabilities = { convert: true, hls: true, hevc: true, hevcUhd: false };
// uhdPC は、BS4K の映像をそのまま再生できる PC。
const uhdPC: Capabilities = { convert: true, hls: true, hevc: true, hevcUhd: true };
// iPhone は MediaSource を持たず、ManagedMediaSource で hls.js を動かす。HEVC は再生できる。
const iphone: Capabilities = { convert: false, hls: true, hevc: true, hevcUhd: true };
const noHevc: Capabilities = { convert: false, hls: true, hevc: false, hevcUhd: false };
const nothing: Capabilities = { convert: false, hls: false, hevc: false, hevcUhd: false };

const presets = [
  { name: '1080p', codec: 'H264', passthrough: false },
  { name: '720p', codec: 'H264', passthrough: false },
  { name: '360p', codec: 'H264', passthrough: false },
  { name: '720p-hevc', codec: 'HEVC', passthrough: false },
  { name: 'source', codec: 'HEVC', passthrough: true },
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
    expect(qualities(capabilities, transcoding, 'HD')).toEqual(want);
  });

  // BS4K の番組では、ブラウザの中で変換できず、映像を変換しない画質は、そのまま再生できる端末にだけ出す (#806)。
  test.each([
    {
      name: 'BS4K の映像をそのまま再生できる PC',
      capabilities: uhdPC,
      format: 'UHD' as const,
      want: ['1080p', '720p', '360p', '720p-hevc', 'source'],
    },
    {
      name: 'BS4K の映像をそのまま再生できない PC',
      capabilities: pc,
      format: 'UHD' as const,
      want: ['1080p', '720p', '360p', '720p-hevc'],
    },
    {
      name: '8K の番組は、どの端末でも再生できない',
      capabilities: uhdPC,
      format: 'SHV' as const,
      want: [],
    },
  ])('$name', ({ capabilities, format, want }) => {
    expect(qualities(capabilities, withKyusu, format)).toEqual(want);
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
    expect(chooseRoute(capabilities, transcoding, requested, 'HD')).toEqual(want);
  });

  test.each([
    {
      name: 'BS4K の映像をそのまま再生できる端末では、映像を変換しない画質を選ぶ',
      capabilities: uhdPC,
      transcoding: withKyusu,
      requested: undefined,
      format: 'UHD' as const,
      want: { kind: 'kyusu', preset: 'source' },
    },
    {
      name: 'BS4K の映像をそのまま再生できない PC では、1080p に変換する',
      capabilities: pc,
      transcoding: withKyusu,
      requested: undefined,
      format: 'UHD' as const,
      want: { kind: 'kyusu', preset: '1080p' },
    },
    {
      name: 'BS4K の映像をそのまま再生できないスマートフォンでは、既定の画質に変換する',
      capabilities: noHevc,
      transcoding: withKyusu,
      requested: undefined,
      format: 'UHD' as const,
      want: { kind: 'kyusu', preset: '720p' },
    },
    {
      name: '2K の番組で覚えたオリジナルは、BS4K の番組では選べないので既定に戻す',
      capabilities: pc,
      transcoding: withKyusu,
      requested: 'original',
      format: 'UHD' as const,
      want: { kind: 'kyusu', preset: '1080p' },
    },
    {
      name: 'BS4K の番組は、kyusu がなければ再生できない',
      capabilities: uhdPC,
      transcoding: withoutKyusu,
      requested: undefined,
      format: 'UHD' as const,
      want: { kind: 'unavailable', reason: 'uhdNoTranscoder' },
    },
    {
      name: '8K の番組は再生できない',
      capabilities: uhdPC,
      transcoding: withKyusu,
      requested: '720p',
      format: 'SHV' as const,
      want: { kind: 'unavailable', reason: 'shv' },
    },
  ])('$name', ({ capabilities, transcoding, requested, format, want }) => {
    expect(chooseRoute(capabilities, transcoding, requested, format)).toEqual(want);
  });
});

describe('hlsURL', () => {
  test.each([
    {
      name: 'TS の URL に、画質と音声ごとのプレイリストのパスを足す',
      streamUrl: '/streams/services/3273601024',
      audio: 1 as const,
      want: '/streams/services/3273601024/hls/720p-hevc/1/playlist.m3u8',
    },
    {
      name: '第 2 音声',
      streamUrl: '/streams/services/3273601024',
      audio: 2 as const,
      want: '/streams/services/3273601024/hls/720p-hevc/2/playlist.m3u8',
    },
    {
      name: 'クエリは、プレイリストのパスの後ろに付け直す',
      streamUrl: '/streams/services/3273601024?token=abc',
      audio: 1 as const,
      want: '/streams/services/3273601024/hls/720p-hevc/1/playlist.m3u8?token=abc',
    },
  ])('$name', ({ streamUrl, audio, want }) => {
    expect(hlsURL(streamUrl, '720p-hevc', audio)).toBe(want);
  });
});

describe('dataBroadcastSource', () => {
  test.each([
    {
      name: 'TS の URL に、データ放送のパスを足し、サービス ID の下 5 桁を program_number にする',
      streamUrl: '/streams/services/3273601024',
      want: { url: '/streams/services/3273601024/data-broadcast', serviceId: 1024 },
    },
    {
      name: 'クエリは、データ放送のパスの後ろに付け直す',
      streamUrl: '/streams/services/400101?token=abc',
      want: { url: '/streams/services/400101/data-broadcast?token=abc', serviceId: 101 },
    },
    { name: 'サービス ID でない', streamUrl: '/streams/services/nhk', want: null },
  ])('$name', ({ streamUrl, want }) => {
    expect(dataBroadcastSource(streamUrl)).toEqual(want);
  });
});

describe('toVideoFormat', () => {
  test.each([
    { value: 'HD', want: 'HD' },
    { value: 'SHV', want: 'SHV' },
    { value: 'UHD', want: 'UHD' },
    // サーバーが新しく、知らない種類を返したら、ブラウザの中での変換を選ばせないよう、BS4K と同じく扱う。
    { value: '%future added value', want: 'UHD' },
  ])('$value', ({ value, want }) => {
    expect(toVideoFormat(value)).toBe(want);
  });
});
