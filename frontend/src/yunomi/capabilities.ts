import type { Capabilities } from './route';

// h264 と hevc は、変換した映像と kyusu の映像のコーデック。
const h264 = 'video/mp4; codecs="avc1.640028"';
const hevc = 'video/mp4; codecs="hvc1.1.6.L120.90"';
// hevcUhd は、BS4K の映像 (HEVC Main10、Level 5.1、2160p) のコーデック (#806)。
const hevcUhd = 'video/mp4; codecs="hvc1.2.4.L153.B0"';

type MediaSourceLike = { isTypeSupported(type: string): boolean };

// detectCapabilities は、この端末で使える再生の仕組みを調べる (設計書 7.1)。
export function detectCapabilities(scope: typeof globalThis = globalThis): Capabilities {
  const mediaSource = (scope as { MediaSource?: MediaSourceLike }).MediaSource;
  const managed = (scope as { ManagedMediaSource?: MediaSourceLike }).ManagedMediaSource;
  const anySource = mediaSource ?? managed;

  return {
    // ブラウザの中での変換は、MediaSource か ManagedMediaSource で H.264 を再生できれば使う。
    // ManagedMediaSource しかない iPhone でも、変換のプレイヤーが ManagedMediaSource で動く (#1256)。
    convert: anySource?.isTypeSupported(h264) ?? false,
    // hls.js は、MediaSource か ManagedMediaSource があれば動く。
    hls: anySource?.isTypeSupported(h264) ?? false,
    hevc: anySource?.isTypeSupported(hevc) ?? false,
    hevcUhd: anySource?.isTypeSupported(hevcUhd) ?? false,
    decode: canDecode(scope),
  };
}

// simdModule は、v128 の命令を 1 つだけ持つ WebAssembly のモジュール。WASM の SIMD が使えるかを、これを検証して調べる。
const simdModule = new Uint8Array([
  0, 97, 115, 109, 1, 0, 0, 0, 1, 5, 1, 96, 0, 1, 123, 3, 2, 1, 0, 10, 10, 1, 8, 0, 65, 0, 253, 15,
  253, 98, 11,
]);

// canDecode は、ブラウザの中で MPEG-2 をデコードする経路 (#1516) を使えるかを返す。WASM の SIMD、worker へ渡せる
// OffscreenCanvas、GPU で描く WebGL2、AudioContext の 4 つがそろっていればよい。GPU のない端末では、WebGL がソフトウェアで
// 描くので (コマの持ち時間を超えるおそれがある)、使わない。
// decodeResults は、端末ごと (scope ごと) に調べた結果。調べるたびに WebGL2 のコンテキストを作らないよう、1 度だけ調べる。
const decodeResults = new WeakMap<object, boolean>();

function canDecode(scope: typeof globalThis): boolean {
  const known = decodeResults.get(scope);
  if (known !== undefined) {
    return known;
  }
  const result = probeDecode(scope);
  decodeResults.set(scope, result);

  return result;
}

function probeDecode(scope: typeof globalThis): boolean {
  try {
    if (
      typeof scope.WebAssembly?.validate !== 'function' ||
      !scope.WebAssembly.validate(simdModule) ||
      typeof scope.OffscreenCanvas !== 'function' ||
      typeof scope.AudioContext !== 'function' ||
      // 時計に、鳴っている音の位置 (getOutputTimestamp) を使う。
      typeof scope.AudioContext.prototype?.getOutputTimestamp !== 'function' ||
      typeof scope.document?.createElement !== 'function'
    ) {
      return false;
    }
    const canvas = scope.document.createElement('canvas');
    if (typeof canvas.transferControlToOffscreen !== 'function') {
      return false;
    }
    const gl = canvas.getContext('webgl2', { failIfMajorPerformanceCaveat: true });
    if (gl === null) {
      return false;
    }
    gl.getExtension('WEBGL_lose_context')?.loseContext();

    return true;
  } catch {
    return false;
  }
}

// prefersManagedMediaSource は、変換のプレイヤーに ManagedMediaSource を使わせるかを返す (#1256)。
// MediaSource がなく、ManagedMediaSource だけがある端末 (iPhone の Safari) で使う。
export function prefersManagedMediaSource(scope: typeof globalThis = globalThis): boolean {
  return !('MediaSource' in scope) && 'ManagedMediaSource' in scope;
}
