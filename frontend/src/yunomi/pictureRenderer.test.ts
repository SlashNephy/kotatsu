import { describe, expect, test } from 'vitest';

import { keepAspect } from './pictureRenderer';

describe('keepAspect', () => {
  const hd = { width: 1920, height: 1080 };

  test.each([
    {
      name: '映像と同じ比率なら、そのまま',
      output: { width: 3840, height: 2160 },
      limit: 16384,
      want: { width: 3840, height: 2160 },
    },
    {
      name: '横だけを拡大した大きさは、縦を映像の比率まで広げる',
      output: { width: 1600, height: 1080 },
      limit: 16384,
      want: { width: 1920, height: 1080 },
    },
    {
      name: '横に引き伸ばす切り抜きの大きさは、縦を広げる',
      output: { width: 2554, height: 1080 },
      limit: 16384,
      want: { width: 2554, height: 1437 },
    },
    {
      name: '広げても上限ちょうどなら、縮めない',
      output: { width: 4096, height: 1080 },
      limit: 4096,
      want: { width: 4096, height: 2304 },
    },
    {
      name: '縦に長い大きさは、横を広げ、上限に収める',
      output: { width: 1920, height: 2160 },
      limit: 2160,
      want: { width: 2160, height: 1215 },
    },
  ])('$name', ({ output, limit, want }) => {
    expect(keepAspect(output, hd, limit)).toEqual(want);
  });
});
