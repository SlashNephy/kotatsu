import { describe, expect, test } from 'vitest';

import { lateCommentWindowSeconds, syncedCommentWait } from './liveSync';

describe('syncedCommentWait', () => {
  // サーバーの今は 10:00:10、映しているコマは放送から 3 秒遅れ (放送の 10:00:07 を映している)。
  const serverNow = Date.parse('2026-11-14T10:00:10.000Z');
  const broadcast = { delay: 3, serverNow };

  test.each([
    {
      name: 'まだ映っていない時刻のコメントは、その時刻を映すまで待つ',
      postedAt: serverNow - 1_000,
      want: 2,
    },
    { name: 'ちょうど映している時刻のコメントは、すぐ流す', postedAt: serverNow - 3_000, want: 0 },
    {
      name: '映している時刻を少し過ぎて届いたコメントは、すぐ流す',
      postedAt: serverNow - 3_000 - lateCommentWindowSeconds * 1_000,
      want: 0,
    },
    {
      name: '映している時刻を大きく過ぎて届いたコメントは、流さない',
      postedAt: serverNow - 3_000 - lateCommentWindowSeconds * 1_000 - 1,
      want: null,
    },
  ])('$name', ({ postedAt, want }) => {
    expect(syncedCommentWait(postedAt, broadcast)).toBe(want);
  });
});
