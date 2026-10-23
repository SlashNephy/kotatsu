import { describe, expect, test } from 'vitest';

import { broadcastGain } from './dataBroadcastAudio';

describe('broadcastGain', () => {
  test.each([
    { name: 'プレイヤーの音量に合わせる', video: { volume: 0.4, muted: false }, want: 0.4 },
    { name: 'ミュートなら鳴らさない', video: { volume: 0.8, muted: true }, want: 0 },
    { name: '音量が 0 なら鳴らさない', video: { volume: 0, muted: false }, want: 0 },
    { name: '映像がなければ鳴らさない', video: null, want: 0 },
  ])('$name', ({ video, want }) => {
    expect(broadcastGain(video)).toBe(want);
  });
});
