import { describe, expect, test } from 'vitest';

import { detectCapabilities } from './capabilities';

// source は、指定したコーデックだけを再生できる MediaSource の代わり。
const source = (...codecs: string[]) => ({
  isTypeSupported: (type: string) => codecs.some((codec) => type.includes(codec)),
});

describe('detectCapabilities', () => {
  test.each([
    {
      name: 'PC のブラウザは、MediaSource で変換もトランスコードも使える',
      scope: { MediaSource: source('avc1', 'hvc1') },
      want: { convert: true, hls: true, hevc: true },
    },
    {
      name: 'iPhone は ManagedMediaSource しかないので、kyusu だけを使う',
      scope: { ManagedMediaSource: source('avc1', 'hvc1') },
      want: { convert: false, hls: true, hevc: true },
    },
    {
      name: 'HEVC を再生できないブラウザ',
      scope: { MediaSource: source('avc1') },
      want: { convert: true, hls: true, hevc: false },
    },
    {
      name: 'MediaSource のないブラウザ',
      scope: {},
      want: { convert: false, hls: false, hevc: false },
    },
  ])('$name', ({ scope, want }) => {
    expect(detectCapabilities(scope as unknown as typeof globalThis)).toEqual(want);
  });
});
