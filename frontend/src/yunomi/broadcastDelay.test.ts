import { describe, expect, test } from 'vitest';

import { broadcastDelaySeconds, type BroadcastClockSample } from './broadcastDelay';

describe('broadcastDelaySeconds', () => {
  // サーバーは 10:00:00.000 に、PCR 1000 秒の TS を受け取った。端末の時計は、サーバーより 5 秒進んでいる。
  const receivedAt = Date.parse('2026-11-14T10:00:00.000Z');
  const clock: BroadcastClockSample = {
    pcr: 1_000,
    receivedAt,
    serverTime: receivedAt + 1_000,
    arrivedAt: receivedAt + 1_000 + 5_000,
  };

  test('映しているコマの PTS と組の PCR の差から、放送の時刻を出し、サーバーの時計で今と比べる', () => {
    // 画面の時刻 0 は PTS 990 秒。画面の 12 秒は PTS 1002 秒なので、放送は 10:00:02。
    // 端末の今 10:00:09 (端末の時計) は、サーバーの時計で 10:00:04。放送から 2 秒。
    const now = receivedAt + 9_000;

    expect(broadcastDelaySeconds(12, 990 * 90_000, clock, now)).toBeCloseTo(2, 6);
  });

  test('PTS が一周しても、近いほうの差で出す', () => {
    const wrap = 2 ** 33 / 90_000;
    const nearWrap: BroadcastClockSample = { ...clock, pcr: wrap - 1 };
    // 画面の 2 秒は、一周した後の PTS 1 秒。組の PCR (一周の 1 秒前) から 2 秒後の放送。
    const now = receivedAt + 5_000 + 3_000;

    expect(broadcastDelaySeconds(2, (wrap - 1) * 90_000, nearWrap, now)).toBeCloseTo(1, 6);
  });
});
