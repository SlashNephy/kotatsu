import { describe, expect, test } from 'vitest';

import { PlaybackTracker } from './usePlaybackReporter';

// play は、壁の時計で start から seconds 秒、放送の時刻 at から等速で再生したことを、1 秒ごとに記録する。
function play(tracker: PlaybackTracker, start: number, at: number, media: number, seconds: number) {
  for (let i = 0; i <= seconds; i++) {
    tracker.sample(start + i * 1000, at + i * 1000, media + i, true, 1);
  }
}

describe('PlaybackTracker', () => {
  test('再生した区間を、放送の時刻で集める', () => {
    const tracker = new PlaybackTracker();
    play(tracker, 0, 1_000_000, 0, 10);
    expect(tracker.pending().intervals).toEqual([{ from: 1_000_000, to: 1_010_000 }]);
  });

  test('一時停止している間は数えない', () => {
    const tracker = new PlaybackTracker();
    play(tracker, 0, 1_000_000, 0, 5);
    // 止めている間は、映像の時刻も放送の時刻も進まない。
    for (let i = 1; i <= 30; i++) {
      tracker.sample(5_000 + i * 1000, 1_005_000, 5, false, 1);
    }
    play(tracker, 35_000, 1_005_000, 5, 5);
    const total = tracker
      .pending()
      .intervals.reduce((sum, interval) => sum + interval.to - interval.from, 0);
    expect(total).toBe(10_000);
  });

  test('最後までシークしただけでは数えない', () => {
    const tracker = new PlaybackTracker();
    play(tracker, 0, 1_000_000, 0, 3);
    // 25 分先へシークした。放送の時刻が、壁の時計より大きく進む。
    tracker.sample(4_000, 2_500_000, 1500, true, 1);
    tracker.sample(5_000, 2_501_000, 1501, true, 1);
    // 飛んだ 25 分は数えず、シークした後に再生した 1 秒だけを足す。
    expect(tracker.pending().intervals).toEqual([
      { from: 1_000_000, to: 1_003_000 },
      { from: 2_500_000, to: 2_501_000 },
    ]);
  });

  test('映像が進まない (読み込みを待っている) 間は数えない', () => {
    const tracker = new PlaybackTracker();
    for (let i = 0; i <= 10; i++) {
      // ライブでは壁の時計の時刻を出しているが、映像は止まったまま。
      tracker.sample(i * 1000, 1_000_000 + i * 1000, 3, true, 1);
    }
    expect(tracker.pending().intervals).toEqual([]);
  });

  test('2 倍速では、放送の時刻で 2 倍の区間を数える', () => {
    const tracker = new PlaybackTracker();
    for (let i = 0; i <= 5; i++) {
      tracker.sample(i * 1000, 1_000_000 + i * 2000, i * 2, true, 2);
    }
    expect(tracker.pending().intervals).toEqual([{ from: 1_000_000, to: 1_010_000 }]);
  });

  test('送り届けた閉じた区間は捨て、続いている区間は残す', () => {
    const tracker = new PlaybackTracker();
    play(tracker, 0, 1_000_000, 0, 3);
    tracker.sample(4_000, 2_000_000, 100, true, 1);
    play(tracker, 4_000, 2_000_000, 100, 3);
    const { intervals, closed } = tracker.pending();
    expect(intervals).toHaveLength(2);
    expect(closed).toBe(1);
    tracker.acknowledge(closed);
    expect(tracker.pending().intervals).toEqual([{ from: 2_000_000, to: 2_003_000 }]);
  });
});
