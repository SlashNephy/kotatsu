import { describe, expect, test } from 'vitest';

import { detectCapabilities, prefersManagedMediaSource } from './capabilities';

// source は、指定したコーデックだけを再生できる MediaSource の代わり。
const source = (...codecs: string[]) => ({
  isTypeSupported: (type: string) => codecs.some((codec) => type.includes(codec)),
});

describe('detectCapabilities', () => {
  test.each([
    {
      name: 'PC のブラウザは、MediaSource で変換もトランスコードも使える',
      scope: { MediaSource: source('avc1', 'hvc1') },
      want: { convert: true, hls: true, hevc: true, hevcUhd: true, decode: false },
    },
    {
      name: 'iPhone は ManagedMediaSource しかないが、変換もトランスコードも使える (#1256)',
      scope: { ManagedMediaSource: source('avc1', 'hvc1') },
      want: { convert: true, hls: true, hevc: true, hevcUhd: true, decode: false },
    },
    {
      name: 'HEVC を再生できないブラウザ',
      scope: { MediaSource: source('avc1') },
      want: { convert: true, hls: true, hevc: false, hevcUhd: false, decode: false },
    },
    {
      name: 'MediaSource のないブラウザ',
      scope: {},
      want: { convert: false, hls: false, hevc: false, hevcUhd: false, decode: false },
    },
    {
      // Linux の Chrome などは、HEVC の 1080p の 8 ビットは復号できても、BS4K の 2160p の 10 ビットは復号できないことがある。
      name: 'BS4K の映像を復号できないブラウザ',
      scope: { MediaSource: source('avc1', 'hvc1.1.') },
      want: { convert: true, hls: true, hevc: true, hevcUhd: false, decode: false },
    },
  ])('$name', ({ scope, want }) => {
    expect(detectCapabilities(scope as unknown as typeof globalThis)).toEqual(want);
  });
});

describe('ブラウザの中でデコードする経路 (#1516) を使えるか', () => {
  // decodeScope は、WASM の SIMD、OffscreenCanvas、AudioContext、WebGL2 の有無を決めた端末の代わり。
  const decodeScope = ({ simd = true, webgl2 = true, offscreen = true } = {}) => ({
    WebAssembly: { validate: () => simd },
    // GPU のない端末と、OffscreenCanvas では 2D しか使えない端末では、WebGL2 を断る。
    OffscreenCanvas: class {
      getContext() {
        return webgl2 ? { getExtension: () => null } : null;
      }
    },
    AudioContext: Object.assign(function AudioContext() {}, {
      prototype: { getOutputTimestamp: () => ({}) },
    }),
    document: {
      createElement: () => ({
        transferControlToOffscreen: offscreen ? () => ({}) : undefined,
        // ページの canvas では、WebGL2 を使える。
        getContext: () => ({ getExtension: () => null }),
      }),
    },
  });

  test.each([
    { name: 'すべてそろえば使える', scope: decodeScope(), want: true },
    { name: 'WASM の SIMD がなければ使えない', scope: decodeScope({ simd: false }), want: false },
    {
      name: 'worker の canvas に GPU で描けなければ使えない',
      scope: decodeScope({ webgl2: false }),
      want: false,
    },
    {
      name: 'canvas を worker へ渡せなければ使えない',
      scope: decodeScope({ offscreen: false }),
      want: false,
    },
  ])('$name', ({ scope, want }) => {
    expect(detectCapabilities(scope as unknown as typeof globalThis).decode).toBe(want);
  });
});

describe('prefersManagedMediaSource', () => {
  test.each([
    {
      name: 'ManagedMediaSource しかない端末 (iPhone) では使う',
      scope: { ManagedMediaSource: source() },
      want: true,
    },
    {
      name: 'MediaSource のある端末では使わない',
      scope: { MediaSource: source(), ManagedMediaSource: source() },
      want: false,
    },
    { name: 'どちらもない端末では使わない', scope: {}, want: false },
  ])('$name', ({ scope, want }) => {
    expect(prefersManagedMediaSource(scope as unknown as typeof globalThis)).toBe(want);
  });
});
