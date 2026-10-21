import { describe, expect, test } from 'vitest';

import { createConnectionTracker, maxRetryDelayMs, retryDelayMs } from './connection';

describe('createConnectionTracker', () => {
  test('つなぎ直している subscription が 1 本でもあれば true で、すべてつながるか、やめれば false に戻る', () => {
    const tracker = createConnectionTracker();
    let notified = 0;
    tracker.subscribe(() => {
      notified++;
    });
    const a = tracker.track();
    const b = tracker.track();
    expect(tracker.isReconnecting()).toBe(false);

    a.reconnecting();
    b.reconnecting();
    expect(tracker.isReconnecting()).toBe(true);
    a.connected();
    expect(tracker.isReconnecting()).toBe(true);
    b.release();
    expect(tracker.isReconnecting()).toBe(false);
    // 知らせるのは、つなぎ直しているかどうかが変わったとき (2 回) と、a がつなぎ直してつながったとき (1 回)。
    expect(notified).toBe(3);
  });

  test('やめた後に届いた知らせは数えない', () => {
    const tracker = createConnectionTracker();
    const a = tracker.track();
    a.reconnecting();
    a.release();
    a.connected();
    expect(tracker.isReconnecting()).toBe(false);
    // やめた後に、つなぎ直しの知らせが遅れて届いても、つなぎ直している状態に戻さない。
    a.reconnecting();
    expect(tracker.isReconnecting()).toBe(false);
  });

  test('知らせるのをやめた listener には知らせない', () => {
    const tracker = createConnectionTracker();
    let notified = 0;
    const unsubscribe = tracker.subscribe(() => {
      notified++;
    });
    unsubscribe();
    tracker.track().reconnecting();
    expect(notified).toBe(0);
  });
});

describe('retryDelayMs', () => {
  test.each([
    { retries: 0, random: 0, want: 1000 + 300 },
    { retries: 1, random: 0, want: 2000 + 300 },
    { retries: 4, random: 0, want: 16_000 + 300 },
    { retries: 5, random: 0, want: maxRetryDelayMs + 300 },
    { retries: 100, random: 0, want: maxRetryDelayMs + 300 },
    { retries: 0, random: 0.9999, want: 1000 + 2999 },
  ])('$retries 回目 (random=$random) は $want ms 待つ', ({ retries, random, want }) => {
    expect(retryDelayMs(retries, random)).toBe(want);
  });
});

describe('createConnectionTracker の recoveries', () => {
  test('つなぎ直していた接続がつながったときだけ数え、そのたびに知らせる', () => {
    const tracker = createConnectionTracker();
    let notified = 0;
    tracker.subscribe(() => {
      notified++;
    });
    const a = tracker.track();

    // 最初からつながった接続は、戻ったとはみなさない。
    a.connected();
    expect(tracker.recoveries()).toBe(0);

    a.reconnecting();
    a.connected();
    expect(tracker.recoveries()).toBe(1);
    // つなぎ直し始めたとき、戻ったとき、つなぎ直している接続がなくなったときに知らせる。
    expect(notified).toBe(3);

    // やめた接続は、つなぎ直していても数えない。
    a.reconnecting();
    a.release();
    a.connected();
    expect(tracker.recoveries()).toBe(1);
  });
});
