import { describe, expect, test } from 'vitest';

import { measureProbe, probeRate, retryAfterMs } from './networkProbe';

describe('probeRate', () => {
  test.each([
    {
      name: '最初の塊が届いてから最後の塊までの間に届いた量で割る (最初の塊までの往復の待ちは数えない)',
      startedAt: 0,
      arrivals: [
        { at: 300, bytes: 100_000 },
        { at: 400, bytes: 125_000 },
        { at: 500, bytes: 125_000 },
      ],
      // 250,000 バイト × 8 ÷ 0.2 秒
      want: 10_000_000,
    },
    {
      name: '塊が 1 つしか来なければ、取り始めからで割る',
      startedAt: 100,
      arrivals: [{ at: 600, bytes: 125_000 }],
      want: 2_000_000,
    },
    { name: '何も届かなければ 0', startedAt: 0, arrivals: [], want: 0 },
  ])('$name', ({ startedAt, arrivals, want }) => {
    expect(probeRate(startedAt, arrivals)).toBe(want);
  });
});

describe('retryAfterMs', () => {
  const now = Date.parse('2026-11-16T12:00:00Z');
  test.each([
    { name: '秒', value: '5', want: 5_000 },
    { name: 'HTTP の日時', value: 'Mon, 16 Nov 2026 12:00:07 GMT', want: 7_000 },
    { name: '過ぎた日時は 0', value: 'Mon, 16 Nov 2026 11:00:00 GMT', want: 0 },
    { name: '無い', value: null, want: null },
    { name: '読めない', value: 'soon', want: null },
  ])('$name', ({ value, want }) => {
    expect(retryAfterMs(value, now)).toBe(want);
  });
});

// stream は、chunks を 1 つずつ返す本文を作る。
function stream(chunks: number[]): ReadableStream<Uint8Array> {
  let i = 0;

  return new ReadableStream({
    pull(controller) {
      const size = chunks[i++];
      if (size === undefined) {
        controller.close();

        return;
      }
      controller.enqueue(new Uint8Array(size));
    },
  });
}

// ticker は、呼ぶたびに 100 ミリ秒進む時計。取り始めが 0 で、チャンクを受け取るたびに 100 ミリ秒後になる。
function ticker(): () => number {
  let calls = 0;

  return () => calls++ * 100;
}

describe('measureProbe', () => {
  test.each([
    {
      name: '測れた速さを返す',
      response: () => new Response(stream([1_000, 125_000, 125_000]), { status: 200 }),
      // 2 つ目からの 250,000 バイトを 0.2 秒で
      want: { kind: 'measured', bps: 10_000_000 },
    },
    {
      name: '5xx は余裕が無かったとみなす',
      response: () => new Response('busy', { status: 503 }),
      want: { kind: 'failed' },
    },
    {
      name: '429 は Retry-After の間は測らない',
      response: () => new Response('', { status: 429, headers: { 'Retry-After': '3' } }),
      want: { kind: 'rateLimited', retryAfterMs: 3_000 },
    },
  ])('$name', async ({ response, want }) => {
    const requests: { url: string; init: RequestInit | undefined }[] = [];
    const outcome = await measureProbe({
      bytes: 300_000,
      timeoutMs: 10_000,
      signal: new AbortController().signal,
      now: ticker(),
      fetch: async (url, init) => {
        requests.push({ url: url instanceof Request ? url.url : url.toString(), init });

        return response();
      },
    });
    expect(outcome).toEqual(want);
    expect(requests.map((request) => [request.url, request.init?.cache])).toEqual([
      ['/internal/streams/probe?bytes=300000', 'no-store'],
    ]);
  });

  test('呼んだ側が打ち切ったら (足りない兆しが出た)、余裕が無かったとみなす', async () => {
    const abort = new AbortController();
    const outcome = measureProbe({
      bytes: 300_000,
      timeoutMs: 10_000,
      signal: abort.signal,
      fetch: (_url, init) =>
        new Promise((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () =>
            reject(new DOMException('aborted', 'AbortError')),
          );
        }),
    });
    abort.abort();
    await expect(outcome).resolves.toEqual({ kind: 'failed' });
  });

  test('試しの打ち切りの時間がたったら、そこまでに届いた量で割る', async () => {
    const outcome = await measureProbe({
      bytes: 4_000_000,
      timeoutMs: 50,
      signal: new AbortController().signal,
      now: ticker(),
      fetch: async (_url, init) => {
        let sent = 0;
        const body = new ReadableStream<Uint8Array>({
          async pull(controller) {
            // 2 つ届いたあとは、打ち切られるまで届かない。
            if (sent >= 2) {
              await new Promise((resolve) => init?.signal?.addEventListener('abort', resolve));
              controller.error(new DOMException('aborted', 'AbortError'));

              return;
            }
            sent += 1;
            controller.enqueue(new Uint8Array(125_000));
          },
        });

        return new Response(body, { status: 200 });
      },
    });
    // 2 つ目の 125,000 バイトを 0.1 秒で
    expect(outcome).toEqual({ kind: 'measured', bps: 10_000_000 });
  });
});
