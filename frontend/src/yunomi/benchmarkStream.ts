import { clockHz, indexStream, StreamLooper } from '../../demo/stream-loop.js';

// 「この端末で試す」(#1771) で流す、合成の映像を繰り返した TS を作る。
// 映像は短く作って配信し (scripts/build-benchmark-streams.sh)、ブラウザの中で、時刻を進めながら必要な長さまで繰り返す。
// 繰り返しは、静的なデモ (#832) と同じ書き換え (demo/stream-loop.js) を使う。

// benchmarkClips は、試す映像。どちらも 1440×1080 の 1080i (トップフィールドが先) で、30 コマ (1.001 秒)。
// interlaced は、59.94 枚の絵をフィールドに分けて載せた映像。film は、23.976 枚の絵を 2:3 プルダウンで載せた映像。
export const benchmarkClips = {
  interlaced: 'benchmark/interlaced.m2ts',
  film: 'benchmark/film.m2ts',
} as const;
export type BenchmarkClip = keyof typeof benchmarkClips;

// clipPeriod は、映像の長さ (90 kHz の単位)。29.97 fps の 30 コマ。プルダウンの 5 コマの周期と、GOP の 15 コマで割り切れる。
export const clipPeriod = 30 * 3003;

// loopClip は、TS の映像 bytes を、seconds 秒ぶん繰り返した TS にする。
export function loopClip(bytes: Uint8Array, seconds: number): Uint8Array<ArrayBuffer> {
  const index = indexStream(bytes, clipPeriod);
  const looper = new StreamLooper(index, { end: seconds * clockHz });
  const chunks: Uint8Array[] = [];
  let length = 0;
  for (;;) {
    const chunk = looper.read(4096);
    if (chunk === null) {
      break;
    }
    chunks.push(chunk);
    length += chunk.length;
  }
  const looped = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) {
    looped.set(chunk, offset);
    offset += chunk.length;
  }

  return looped;
}

// createBenchmarkStream は、試す映像を読み、seconds 秒ぶん繰り返した TS の URL (blob:) を返す。使い終えたら revoke する。
export async function createBenchmarkStream(
  clip: BenchmarkClip,
  seconds: number,
  signal?: AbortSignal,
): Promise<string> {
  const response = await fetch(`${import.meta.env.BASE_URL}${benchmarkClips[clip]}`, { signal });
  if (!response.ok) {
    throw new Error(`failed to fetch the benchmark stream: ${response.status}`);
  }
  const bytes = new Uint8Array(await response.arrayBuffer());

  return URL.createObjectURL(new Blob([loopClip(bytes, seconds)], { type: 'video/mp2t' }));
}
