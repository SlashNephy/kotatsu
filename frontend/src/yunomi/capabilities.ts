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
  };
}

// prefersManagedMediaSource は、変換のプレイヤーに ManagedMediaSource を使わせるかを返す (#1256)。
// MediaSource がなく、ManagedMediaSource だけがある端末 (iPhone の Safari) で使う。
export function prefersManagedMediaSource(scope: typeof globalThis = globalThis): boolean {
  return !('MediaSource' in scope) && 'ManagedMediaSource' in scope;
}
