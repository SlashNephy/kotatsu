import { afterEach, describe, expect, test, vi } from 'vitest';

import {
  chooseRoute,
  decoded,
  forked,
  dataBroadcastEndpoint,
  dataBroadcastSource,
  hlsURL,
  decodeStreamEndpoint,
  vodDataBroadcastURL,
  original,
  qualities,
  toVideoFormat,
  type Capabilities,
  type Transcoding,
} from './route';

// pc は、HEVC の 2160p の 10 ビットを復号できない PC (Linux の Chrome など)。
// GPU があり、ブラウザの中で MPEG-2 をデコードできる (#1516)。
const pc: Capabilities = { convert: true, hls: true, hevc: true, hevcUhd: false, decode: true };
// uhdPC は、BS4K の映像をそのまま再生できる PC。
const uhdPC: Capabilities = { convert: true, hls: true, hevc: true, hevcUhd: true, decode: false };
// iPhone は MediaSource を持たず、ManagedMediaSource で hls.js を動かす。HEVC は再生できる。
const iphone: Capabilities = {
  convert: false,
  hls: true,
  hevc: true,
  hevcUhd: true,
  decode: false,
};
const noHevc: Capabilities = {
  convert: false,
  hls: true,
  hevc: false,
  hevcUhd: false,
  decode: false,
};
const nothing: Capabilities = {
  convert: false,
  hls: false,
  hevc: false,
  hevcUhd: false,
  decode: false,
};

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

describe('ブラウザの中でデコードする経路 (#1516)', () => {
  test('選ばせる画面で、HD の番組なら、オリジナルの次に並べる', () => {
    expect(qualities(pc, withKyusu, 'HD', { decode: true }).slice(0, 2)).toEqual([
      original,
      decoded,
    ]);
  });

  test('選ばせない画面と、BS4K の番組と、デコードできない端末では並べない', () => {
    expect(qualities(pc, withKyusu, 'HD')).not.toContain(decoded);
    expect(qualities(pc, withKyusu, 'UHD', { decode: true })).not.toContain(decoded);
    expect(qualities(uhdPC, withKyusu, 'HD', { decode: true })).not.toContain(decoded);
  });

  test('手で選んだときだけ使い、自動では選ばない', () => {
    expect(chooseRoute(pc, withKyusu, decoded, 'HD', { decode: true })).toEqual({
      kind: 'decode',
    });
    expect(chooseRoute(pc, withKyusu, undefined, 'HD', { decode: true })).toEqual({
      kind: 'convert',
    });
  });

  test.each([
    {
      name: 'kyusu がいなければ、再生できない',
      transcoding: withoutKyusu,
      want: { kind: 'unavailable', reason: 'noTranscoder' },
    },
    {
      name: 'kyusu の既定の画質がなければ、ほかの kyusu の画質を選ぶ',
      transcoding: { available: true, presets: presets.filter((preset) => preset.name !== '720p') },
      want: { kind: 'kyusu', preset: '1080p' },
    },
  ])('ブラウザの中で変換できない端末でも、自動では選ばない: $name', ({ transcoding, want }) => {
    const decodeOnly: Capabilities = { ...nothing, hls: true, decode: true };

    expect(chooseRoute(decodeOnly, transcoding, undefined, 'HD', { decode: true })).toEqual(want);
  });

  test('選ばせない画面で覚えていたら、既定の経路に戻す', () => {
    expect(chooseRoute(pc, withKyusu, decoded, 'HD')).toEqual({ kind: 'convert' });
  });
});

describe('フォークのプレイヤーの経路 (#2287)', () => {
  // securePC は、安全な文脈 (https か localhost) で開いた、フォークでデコードできる PC。
  const securePC: Capabilities = { ...pc, fork: true };

  test('選ばせる画面で、HD の番組なら、デコードする経路の次に並べる', () => {
    expect(qualities(securePC, withKyusu, 'HD', { decode: true }).slice(0, 3)).toEqual([
      original,
      decoded,
      forked,
    ]);
  });

  test('安全な文脈でない端末と、選ばせない画面と、BS4K の番組では並べない', () => {
    expect(qualities(pc, withKyusu, 'HD', { decode: true })).not.toContain(forked);
    expect(qualities(securePC, withKyusu, 'HD')).not.toContain(forked);
    expect(qualities(securePC, withKyusu, 'UHD', { decode: true })).not.toContain(forked);
  });

  test('手で選んだときだけ、フォークで変換の経路を使い、自動では選ばない', () => {
    expect(chooseRoute(securePC, withKyusu, forked, 'HD', { decode: true })).toEqual({
      kind: 'convert',
      engine: 'fork',
    });
    expect(chooseRoute(securePC, withKyusu, undefined, 'HD', { decode: true })).toEqual({
      kind: 'convert',
    });
  });

  test('安全な文脈でない端末で、保存した画質がフォークなら、既定の経路に戻す', () => {
    expect(chooseRoute(pc, withKyusu, forked, 'HD', { decode: true })).toEqual({ kind: 'convert' });
  });
});

describe('hlsURL', () => {
  test.each([
    {
      name: 'TS の URL に、画質と音声ごとのプレイリストのパスを足す',
      streamUrl: '/internal/streams/services/3273601024',
      audio: 1 as const,
      want: '/internal/streams/services/3273601024/hls/720p-hevc/1/playlist.m3u8',
    },
    {
      name: '第 2 音声',
      streamUrl: '/internal/streams/services/3273601024',
      audio: 2 as const,
      want: '/internal/streams/services/3273601024/hls/720p-hevc/2/playlist.m3u8',
    },
    {
      name: 'クエリは、プレイリストのパスの後ろに付け直す',
      streamUrl: '/internal/streams/services/3273601024?token=abc',
      audio: 1 as const,
      want: '/internal/streams/services/3273601024/hls/720p-hevc/1/playlist.m3u8?token=abc',
    },
  ])('$name', ({ streamUrl, audio, want }) => {
    expect(hlsURL(streamUrl, '720p-hevc', audio)).toBe(want);
  });
});

describe('dataBroadcastSource', () => {
  test.each([
    {
      name: 'TS の URL に、データ放送のパスを足し、サービス ID の下 5 桁を program_number にする',
      streamUrl: '/internal/streams/services/3273601024',
      want: { url: '/internal/streams/services/3273601024/data-broadcast', serviceId: 1024 },
    },
    {
      name: 'クエリは、データ放送のパスの後ろに付け直す',
      streamUrl: '/internal/streams/services/400101?token=abc',
      want: { url: '/internal/streams/services/400101/data-broadcast?token=abc', serviceId: 101 },
    },
    { name: 'サービス ID でない', streamUrl: '/internal/streams/services/nhk', want: null },
  ])('$name', ({ streamUrl, want }) => {
    expect(dataBroadcastSource(streamUrl)).toEqual(want);
  });
});

describe('dataBroadcastEndpoint', () => {
  test.each([
    {
      name: 'データ放送のエンドポイントは、そのまま読む',
      url: '/internal/streams/services/3273601024/data-broadcast',
      want: '/internal/streams/services/3273601024/data-broadcast',
    },
    {
      name: 'クエリも付けたまま読む',
      url: '/internal/streams/services/400101/data-broadcast?token=abc',
      want: '/internal/streams/services/400101/data-broadcast?token=abc',
    },
    {
      name: '録画のデータ放送のエンドポイントも、読み始める位置を付けたまま読む (#2270)',
      url: '/internal/streams/recordings/01a21890-0175-7738-9d6d-a5327f94612c/data-broadcast?start=60.0#3',
      want: '/internal/streams/recordings/01a21890-0175-7738-9d6d-a5327f94612c/data-broadcast?start=60.0',
    },
    {
      name: 'ライブラリのファイルのデータ放送のエンドポイントも読む',
      url: '/internal/streams/library/bWFpbgBhLnRz/data-broadcast?start=0.0',
      want: '/internal/streams/library/bWFpbgBhLnRz/data-broadcast?start=0.0',
    },
    {
      name: '録画型の再生元の問い合わせは、読み始める位置と番組の始まりのほかは渡さない',
      url: '/internal/streams/timeshift/3273601024/data-broadcast?from=2026-11-28T00%3A00%3A00Z&start=12.3&next=https%3A%2F%2Fevil.example#2',
      want: '/internal/streams/timeshift/3273601024/data-broadcast?from=2026-11-28T00%3A00%3A00.000Z&start=12.3',
    },
    {
      name: '数でない読み始める位置は渡さない',
      url: '/internal/streams/recordings/01a21890-0175-7738-9d6d-a5327f94612c/data-broadcast?start=abc',
      want: '/internal/streams/recordings/01a21890-0175-7738-9d6d-a5327f94612c/data-broadcast',
    },
    {
      name: '録画型の再生元の、ほかの種類のパスは読まない',
      url: '/internal/streams/captures/1/data-broadcast',
      want: null,
    },
    {
      name: 'ほかのホストは読まない',
      url: 'https://evil.example/internal/streams/services/1/data-broadcast',
      want: null,
    },
    {
      name: 'ホストを省いた URL も読まない',
      url: '//evil.example/internal/streams/services/1/data-broadcast',
      want: null,
    },
    {
      name: 'ほかのパスは読まない',
      url: '/internal/streams/services/1/data-broadcast/../../../api',
      want: null,
    },
    {
      name: 'サービス ID でない',
      url: '/internal/streams/services/nhk/data-broadcast',
      want: null,
    },
  ])('$name', ({ url, want }) => {
    expect(dataBroadcastEndpoint(url)).toBe(want);
  });
});

describe('decodeStreamEndpoint', () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  test.each([
    {
      name: 'ライブ視聴の TS は、そのまま読む',
      url: '/internal/streams/services/3273601024?session=1',
      want: '/internal/streams/services/3273601024?session=1',
    },
    {
      name: '問い合わせのないライブ視聴の TS も読む',
      url: '/internal/streams/services/3273601024',
      want: '/internal/streams/services/3273601024',
    },
    {
      name: '録画の TS は、読み始める位置を付けたまま読む。fragment は送らないので外す (#2166)',
      url: '/internal/streams/recordings/01a21890-0175-7738-9d6d-a5327f94612c?start=60.0#1',
      want: '/internal/streams/recordings/01a21890-0175-7738-9d6d-a5327f94612c?start=60.0',
    },
    {
      name: '録画の TS の、読み始める位置のほかの問い合わせは渡さない',
      url: '/internal/streams/recordings/01a21890-0175-7738-9d6d-a5327f94612c?start=60&next=https%3A%2F%2Fevil.example',
      want: '/internal/streams/recordings/01a21890-0175-7738-9d6d-a5327f94612c?start=60.0',
    },
    {
      name: '録画の ID が UUID でなければ読まない',
      url: '/internal/streams/recordings/../../../api',
      want: null,
    },
    {
      name: 'ほかのホストは読まない',
      url: 'https://evil.example/internal/streams/services/1',
      want: null,
    },
    {
      name: 'ほかのパスは読まない',
      url: '/internal/streams/services/1/../../../api',
      want: null,
    },
    {
      name: 'デモのビルドでなければ、デモの映像は読まない',
      url: '/demo/stream.m2ts?session=1',
      want: null,
    },
  ])('$name', ({ url, want }) => {
    expect(decodeStreamEndpoint(url)).toBe(want);
  });

  // 静的なデモ (#832) は、どの局もデモの映像を流す。ブラウザの中でデコードする経路でも読めるようにする (#2157)。
  test.each([
    {
      name: 'デモの映像は、問い合わせを付けたまま読む',
      url: '/demo/stream.m2ts?session=1',
      want: '/demo/stream.m2ts?session=1',
    },
    {
      name: '問い合わせのないデモの映像も読む',
      url: '/demo/stream.m2ts',
      want: '/demo/stream.m2ts',
    },
    {
      name: 'ライブ視聴の TS も読む',
      url: '/internal/streams/services/1?session=1',
      want: '/internal/streams/services/1?session=1',
    },
    {
      name: 'ほかのホストのデモの映像は読まない',
      url: 'https://evil.example/demo/stream.m2ts?session=1',
      want: null,
    },
    {
      name: 'デモの映像の後ろにパスを足したものは読まない',
      url: '/demo/stream.m2ts/../../api',
      want: null,
    },
  ])('デモのビルドで、$name', ({ url, want }) => {
    vi.stubEnv('MODE', 'demo');
    expect(decodeStreamEndpoint(url)).toBe(want);
  });
});

describe('vodDataBroadcastURL', () => {
  test.each([
    {
      name: '録画は、データ放送の配信のパスに読み始める位置を付ける',
      streamUrl: '/internal/streams/recordings/01a21890-0175-7738-9d6d-a5327f94612c',
      start: 60,
      want: '/internal/streams/recordings/01a21890-0175-7738-9d6d-a5327f94612c/data-broadcast?start=60.0',
    },
    {
      name: 'タイムシフトのバッファは、番組の始まりの指定を残す',
      streamUrl: '/internal/streams/timeshift/3273601024?from=2026-11-28T00%3A00%3A00Z',
      start: 12.34,
      want: '/internal/streams/timeshift/3273601024/data-broadcast?from=2026-11-28T00%3A00%3A00Z&start=12.3',
    },
  ])('$name', ({ streamUrl, start, want }) => {
    expect(vodDataBroadcastURL(streamUrl, start)).toBe(want);
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

describe('decodeStreamEndpoint の blob: の URL (#2166)', () => {
  const origin = 'https://localhost:9090';
  test.each([
    {
      name: '同じオリジンの blob: は、オリジンと UUID から組み直し、fragment を外す',
      url: `${'blob:'}${origin}/0b7f6c1e-1234-4abc-8def-0123456789ab#3`,
      want: `${'blob:'}${origin}/0b7f6c1e-1234-4abc-8def-0123456789ab`,
    },
    {
      name: '同じオリジンでも、UUID でなければ読まない',
      url: `${'blob:'}${origin}/../internal/api`,
      want: null,
    },
    {
      name: 'ほかのオリジンの blob: は読まない',
      url: 'blob:https://evil.example/0b7f6c1e-1234-4abc-8def-0123456789ab',
      want: null,
    },
  ])('$name', ({ url, want }) => {
    expect(decodeStreamEndpoint(url, origin)).toBe(want);
  });
});
