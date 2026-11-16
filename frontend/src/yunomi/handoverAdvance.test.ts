import { describe, expect, test } from 'vitest';

import { advanceBeforeHandover, type HandoverClock, type HandoverMedia } from './handoverAdvance';

// ranges は、[始まり, 終わり] の組から TimeRanges を作る。
function ranges(...spans: Array<[number, number]>): TimeRanges {
  return {
    length: spans.length,
    start: (i: number) => spans[i]![0],
    end: (i: number) => spans[i]![1],
  };
}

// fakeClock は、sleep で時計を進め、そのたびに onSleep を呼ぶ時計。
function fakeClock(onSleep: (now: number) => void = () => {}): HandoverClock & { slept: number } {
  let now = 0;
  const clock = {
    slept: 0,
    now: () => now,
    sleep: (ms: number) => {
      now += ms;
      clock.slept += ms;
      onSleep(now);

      return Promise.resolve();
    },
  };

  return clock;
}

describe('advanceBeforeHandover', () => {
  test('進める先が読み込んだ範囲にあれば、そこへ進める', async () => {
    const video: HandoverMedia = { currentTime: 1, buffered: ranges([1, 20]), seeking: false };
    const advanced = await advanceBeforeHandover(
      video,
      () => 6,
      () => false,
      fakeClock(),
    );
    expect(advanced).toBe(true);
    expect(video.currentTime).toBe(6);
  });

  test('進める先が分からなければ、進めない', async () => {
    const video: HandoverMedia = { currentTime: 1, buffered: ranges([1, 20]), seeking: false };
    const advanced = await advanceBeforeHandover(
      video,
      () => null,
      () => false,
      fakeClock(),
    );
    expect(advanced).toBe(false);
    expect(video.currentTime).toBe(1);
  });

  test('進める先が今の時刻より手前なら、戻さない', async () => {
    const video: HandoverMedia = { currentTime: 5, buffered: ranges([1, 20]), seeking: false };
    const advanced = await advanceBeforeHandover(
      video,
      () => 3,
      () => false,
      fakeClock(),
    );
    expect(advanced).toBe(false);
    expect(video.currentTime).toBe(5);
  });

  test('読み込んだ範囲に入るまで待ち、入ったら求め直した先へ進める', async () => {
    const video = { currentTime: 1, buffered: ranges([1, 3]), seeking: false };
    // 今の映像は流れ続けるので、待つ間に進める先も後ろへずれる。
    let target = 6;
    const clock = fakeClock((now) => {
      target = 6 + now / 1000;
      if (now >= 1000) {
        video.buffered = ranges([1, 20]);
      }
    });
    const advanced = await advanceBeforeHandover(
      video,
      () => target,
      () => false,
      clock,
    );
    expect(advanced).toBe(true);
    expect(video.currentTime).toBe(7);
  });

  test('上限まで読み込んだ範囲に入らなければ、進めずに終える', async () => {
    const video: HandoverMedia = { currentTime: 1, buffered: ranges([1, 3]), seeking: false };
    const clock = fakeClock();
    const advanced = await advanceBeforeHandover(
      video,
      () => 6,
      () => false,
      clock,
    );
    expect(advanced).toBe(false);
    expect(video.currentTime).toBe(1);
    expect(clock.slept).toBe(5_000);
  });

  test('取り消されたら、待たずに終える', async () => {
    const video: HandoverMedia = { currentTime: 1, buffered: ranges([1, 3]), seeking: false };
    let cancelled = false;
    const clock = fakeClock(() => {
      cancelled = true;
    });
    const advanced = await advanceBeforeHandover(
      video,
      () => 6,
      () => cancelled,
      clock,
    );
    expect(advanced).toBe(false);
    expect(clock.slept).toBe(200);
  });

  test('進めたら、その位置のコマが出るまで待つ', async () => {
    const video = { currentTime: 1, buffered: ranges([1, 20]), seeking: false };
    let seekedAt: number | null = null;
    const clock = fakeClock((now) => {
      if (now >= 150) {
        video.seeking = false;
        seekedAt ??= now;
      }
    });
    const media: HandoverMedia = {
      get currentTime() {
        return video.currentTime;
      },
      set currentTime(t: number) {
        video.currentTime = t;
        video.seeking = true;
      },
      get buffered() {
        return video.buffered;
      },
      get seeking() {
        return video.seeking;
      },
    };
    const advanced = await advanceBeforeHandover(
      media,
      () => 6,
      () => false,
      clock,
    );
    expect(advanced).toBe(true);
    expect(video.seeking).toBe(false);
    expect(seekedAt).toBe(150);
  });
});
