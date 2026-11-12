import { describe, expect, test } from 'vitest';

import { coverRect } from './playerCover';

describe('coverRect', () => {
  test.each([
    {
      name: '16:9 の枠いっぱいに映るなら、映像の要素と同じ矩形',
      container: { left: 16, top: 112, width: 782, height: 440 },
      video: { left: 16, top: 112, width: 782, height: 440 },
      size: [1920, 1080],
      ratio: 1,
      want: { left: 0, top: 0, width: 782, height: 440 },
    },
    {
      // 4:3 の映像は、左右に黒い帯 (レターボックス) が付く。帯は覆わない。
      name: '帯を除いた、映像が実際に出ている矩形に合わせる',
      container: { left: 0, top: 0, width: 800, height: 450 },
      video: { left: 0, top: 0, width: 800, height: 450 },
      size: [1440, 1080],
      ratio: 1,
      want: { left: 100, top: 0, width: 600, height: 450 },
    },
    {
      // 端数のある矩形は、端末の画素の境目へ外側に丸め、映像の縁が蓋の外に覗かないようにする。
      name: '端末の画素の境目へ外側に丸める',
      container: { left: 0, top: 0, width: 782, height: 439.875 },
      video: { left: 0.3, top: 0.0625, width: 781.5, height: 439.6 },
      size: [1920, 1080],
      ratio: 2,
      want: { left: 0, top: 0, width: 782, height: 440 },
    },
  ])('$name', ({ container, video, size, ratio, want }) => {
    const rect = coverRect(container, video, size[0]!, size[1]!, ratio);
    expect(rect.left).toBeCloseTo(want.left);
    expect(rect.top).toBeCloseTo(want.top);
    expect(rect.width).toBeCloseTo(want.width);
    expect(rect.height).toBeCloseTo(want.height);
  });
});
