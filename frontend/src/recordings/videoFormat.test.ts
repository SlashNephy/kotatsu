import { describe, expect, test } from 'vitest';

import { distinctVideoLabels, videoFormatLabel } from './videoFormat';

describe('videoFormatLabel', () => {
  test.each([
    {
      name: 'インターレースの MPEG-2',
      video: {
        codec: 'mpeg2',
        width: 1440,
        height: 1080,
        interlaced: true,
        frameRate: 30000 / 1001,
      },
      want: 'MPEG-2 1440×1080i 29.97fps',
    },
    {
      name: 'フレームレートの分からない HEVC',
      video: { codec: 'hevc', width: 3840, height: 2160, interlaced: false, frameRate: null },
      want: 'HEVC 3840×2160p',
    },
  ])('$name', ({ video, want }) => {
    expect(videoFormatLabel(video)).toBe(want);
  });
});

describe('distinctVideoLabels', () => {
  test('同じ形式は 1 つにまとめ、読めなかったセグメントは飛ばす', () => {
    const hd = {
      codec: 'mpeg2',
      width: 1440,
      height: 1080,
      interlaced: true,
      frameRate: 30000 / 1001,
    };
    const sd = {
      codec: 'mpeg2',
      width: 720,
      height: 480,
      interlaced: true,
      frameRate: 30000 / 1001,
    };
    expect(distinctVideoLabels([hd, null, hd, sd])).toEqual([
      'MPEG-2 1440×1080i 29.97fps',
      'MPEG-2 720×480i 29.97fps',
    ]);
  });
});
