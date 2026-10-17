import { describe, expect, test } from 'vitest';

import { applyVolume, currentVolume, lowerBoost, splitVolume } from './volumeBoost';

describe('splitVolume', () => {
  test.each([
    { name: '100% までは video.volume だけで変える', volume: 0.6, want: { element: 0.6, gain: 1 } },
    { name: '100% を超える分は GainNode で上げる', volume: 1.5, want: { element: 1, gain: 1.5 } },
    { name: '上限 (200%) で止める', volume: 3, want: { element: 1, gain: 2 } },
    { name: '0 より下げない', volume: -1, want: { element: 0, gain: 1 } },
  ])('$name', ({ volume, want }) => {
    expect(splitVolume(volume)).toEqual(want);
  });
});

describe('applyVolume', () => {
  // jsdom には AudioContext がないので、GainNode を挟めないときと同じく、100% で鳴らして上げたい倍率を覚えておく。
  test('WebAudio を使えないときは 100% で鳴らし、上げたい音量を覚えておく', () => {
    const video = document.createElement('video');

    applyVolume(video, 1.5);
    expect(video.volume).toBe(1);
    expect(currentVolume(video)).toBe(1.5);

    // ほかの経路で 1 より下げたら、上げていた分を戻す。
    video.volume = 0.4;
    lowerBoost(video);
    expect(currentVolume(video)).toBe(0.4);
  });
});
