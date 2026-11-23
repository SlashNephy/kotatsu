import { describe, expect, test } from 'vitest';

import { AudioTimeline } from './audioTimeline';

// 48kHz の 1024 サンプルは、PTS で 1920、秒で 0.021333…。
const chunk = 1920;
const chunkSeconds = 1024 / 48_000;

describe('AudioTimeline', () => {
  test('最初の音は「今 + 余裕」に、続く音は隙間なく予約し、鳴っている PTS を出す', () => {
    const timeline = new AudioTimeline({ lead: 0.1 });

    expect(timeline.ptsAt(1)).toBeNull();
    expect(timeline.schedule(9_000, chunk, 1)).toEqual({ when: 1.1, offset: 0 });
    const next = timeline.schedule(9_000 + chunk, chunk, 1.01);

    expect(next?.when).toBeCloseTo(1.1 + chunkSeconds);
    expect(timeline.ptsAt(1.1 + 0.5)).toBeCloseTo(9_000 + 0.5 * 90_000);
    expect(timeline.bufferedUntil).toBe(9_000 + 2 * chunk);
  });

  test('隙間があれば、時刻の対応を保って、隙間の分だけ空けて鳴らす', () => {
    const timeline = new AudioTimeline({ lead: 0.1 });
    timeline.schedule(0, chunk, 0);
    const after = timeline.schedule(9_000, chunk, 0.01);

    expect(after?.when).toBeCloseTo(0.1 + 0.1);
  });

  test('前より古い音は捨て、重なる音は頭を切り落とす', () => {
    const timeline = new AudioTimeline({ lead: 0.1 });
    timeline.schedule(0, chunk, 0);

    expect(timeline.schedule(0, chunk, 0.01)).toBeNull();
    const overlap = timeline.schedule(chunk - 900, chunk, 0.01);
    expect(overlap?.offset).toBeCloseTo(0.01);
    expect(overlap?.when).toBeCloseTo(0.1 + chunkSeconds);
  });

  test('番を過ぎた音は、今から鳴らし直し、鳴らし直した回数を数える', () => {
    const timeline = new AudioTimeline({ lead: 0.1 });
    timeline.schedule(0, chunk, 0);
    // 次の音が届く前に、予約した音を鳴らし終えた (途切れた)。
    const late = timeline.schedule(chunk, chunk, 1);

    expect(late).toEqual({ when: 1.1, offset: 0 });
    expect(timeline.restarts).toBe(1);
    expect(timeline.ptsAt(1.1)).toBe(chunk);
  });

  test('reset したら、次の音から鳴らし始め直す', () => {
    const timeline = new AudioTimeline({ lead: 0.1 });
    timeline.schedule(0, chunk, 0);
    timeline.reset();

    expect(timeline.ptsAt(0.2)).toBeNull();
    expect(timeline.schedule(90_000, chunk, 5)).toEqual({ when: 5.1, offset: 0 });
  });
});
