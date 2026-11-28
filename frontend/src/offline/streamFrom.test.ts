import { describe, expect, test } from 'vitest';

import { firstPCR, streamFrom } from './streamFrom';

// tsWithPCR は、seconds(i) 秒の PCR を 10 パケットごとに入れた TS を作る。ほかのパケットは、ペイロードを空にした映像のパケット。
function tsWithPCR(packets: number, seconds: (index: number) => number): Blob {
  const bytes = new Uint8Array(packets * 188);
  for (let i = 0; i < packets; i += 1) {
    const p = i * 188;
    bytes[p] = 0x47;
    bytes[p + 1] = 0x01;
    bytes[p + 2] = 0x00;
    if (i % 10 === 0) {
      // アダプテーションフィールドだけのパケットに、PCR を入れる。
      bytes[p + 3] = 0x20;
      bytes[p + 4] = 183;
      bytes[p + 5] = 0x10;
      const base = Math.round(seconds(i) * 90_000);
      bytes[p + 6] = Math.floor(base / 2 ** 25) & 0xff;
      bytes[p + 7] = Math.floor(base / 2 ** 17) & 0xff;
      bytes[p + 8] = Math.floor(base / 2 ** 9) & 0xff;
      bytes[p + 9] = Math.floor(base / 2) & 0xff;
      bytes[p + 10] = (base % 2) << 7;
    } else {
      bytes[p + 3] = 0x10;
    }
  }

  return new Blob([bytes]);
}

const recordedFrom = '2026-11-28T00:00:00Z';

describe('firstPCR', () => {
  test('パケットの境目でない所から探しても、次の PCR の位置と時刻を返す', async () => {
    const video = tsWithPCR(100, (i) => 100 + i * 0.01);

    const found = await firstPCR(video, 188 * 3 + 50);

    expect(found).toEqual({ offset: 188 * 10, seconds: 100.1 });
  });
});

describe('streamFrom', () => {
  // 合成する TS は、1 パケットを 1 ミリ秒前後にする (放送の 16 Mbps は、1 パケットが約 0.1 ミリ秒)。
  test.each([
    {
      name: 'ビットレートが揺れても、求めた位置の少し手前から切り出す',
      // 前半はパケットあたり 1 ミリ秒、後半は 0.2 ミリ秒 (ビットレートが 5 倍) で進む。
      seconds: (i: number) => 50 + (i < 50_000 ? i * 0.001 : 50 + (i - 50_000) * 0.0002),
      gaps: [],
      target: 55,
    },
    {
      name: '途切れた区間の分は、位置に数えない',
      // 録り始めから 30 秒の所で 60 秒途切れ、PCR はその分だけ進む。
      seconds: (i: number) => 50 + (i < 30_000 ? i * 0.001 : 90 + (i - 30_000) * 0.001),
      gaps: [{ from: '2026-11-28T00:00:30Z', to: '2026-11-28T00:01:30Z' }],
      target: 60,
    },
  ])('$name', async ({ seconds, gaps, target }) => {
    const video = tsWithPCR(100_000, seconds);

    const { blob, at } = await streamFrom({ video, recordedFrom, gaps }, target);

    // 1 秒手前を目指す。切り出す位置は、PCR を探す幅 (2000 パケット) の分だけ手前にずれることがある。
    expect(at).toBeLessThanOrEqual(target - 1);
    expect(at).toBeGreaterThan(target - 1 - 2);
    expect(blob.size).toBeLessThan(video.size);
    const head = await firstPCR(blob, 0);
    expect(head?.offset).toBe(0);
  });

  test('頭の近くなら、切り出さずに頭から読む', async () => {
    const video = tsWithPCR(1000, (i) => i * 0.01);

    const { blob, at } = await streamFrom({ video, recordedFrom, gaps: [] }, 0.5);

    expect(blob).toBe(video);
    expect(at).toBe(0);
  });
});
