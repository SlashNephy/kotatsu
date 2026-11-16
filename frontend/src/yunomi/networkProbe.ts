// 試しのデータを取り、回線の速さを測る (#1513、設計書 7.3)。

import type { ProbeOutcome } from './networkQuality';

// probePath は、tenban の試しのデータの経路。
export const probePath = '/internal/streams/probe';

// Arrival は、届いた塊。at は届いた時刻 (ミリ秒)。
export type Arrival = { at: number; bytes: number };

// probeRate は、試しのデータの速さ (bps) を返す。最初の塊が届いてから最後の塊までの間に届いたバイト数で割る。
// 最初の塊までは往復の待ちで、回線の速さではないため。塊が 1 つしか来なければ、取り始め (startedAt) からで割る。
// 何も届かなければ 0。
export function probeRate(startedAt: number, arrivals: readonly Arrival[]): number {
  const first = arrivals[0];
  const last = arrivals.at(-1);
  if (first === undefined || last === undefined) {
    return 0;
  }
  if (arrivals.length === 1 || last.at <= first.at) {
    const total = arrivals.reduce((sum, arrival) => sum + arrival.bytes, 0);
    const elapsed = last.at - startedAt;

    return elapsed > 0 ? (total * 8 * 1_000) / elapsed : 0;
  }
  const after = arrivals.slice(1).reduce((sum, arrival) => sum + arrival.bytes, 0);

  return (after * 8 * 1_000) / (last.at - first.at);
}

// retryAfterMs は、Retry-After (秒か HTTP の日時) を、now (Unix ミリ秒) からのミリ秒にする。読めなければ null。
export function retryAfterMs(value: string | null, now: number): number | null {
  if (value === null || value.trim() === '') {
    return null;
  }
  const seconds = Number(value);
  if (Number.isFinite(seconds)) {
    return seconds >= 0 ? seconds * 1_000 : null;
  }
  const at = Date.parse(value);

  return Number.isNaN(at) ? null : Math.max(at - now, 0);
}

export type ProbeOptions = {
  bytes: number;
  // timeoutMs は、試しの打ち切り。打ち切ったら、そこまでに届いた量で割る。
  timeoutMs: number;
  // signal は、呼んだ側が打ち切るためのもの。打ち切ったら (足りない兆しが出た)、余裕が無かったとみなす。
  signal: AbortSignal;
  fetch?: typeof fetch;
  // now は、時刻 (ミリ秒) を返す。
  now?: () => number;
};

// measureProbe は、試しのデータを取って速さを測る。5xx と、呼んだ側の打ち切りと、通信の失敗は、余裕が無かったとみなす (failed)。
// 429 は、Retry-After の間は測らないよう rateLimited を返す。
export async function measureProbe(options: ProbeOptions): Promise<ProbeOutcome> {
  const fetchFn = options.fetch ?? fetch;
  const now = options.now ?? (() => performance.now());
  const controller = new AbortController();
  const abort = () => controller.abort();
  options.signal.addEventListener('abort', abort);
  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, options.timeoutMs);
  const startedAt = now();
  const arrivals: Arrival[] = [];
  try {
    const response = await fetchFn(`${probePath}?bytes=${options.bytes}`, {
      cache: 'no-store',
      credentials: 'same-origin',
      signal: controller.signal,
    });
    if (response.status === 429) {
      await response.body?.cancel().catch(() => {});

      return {
        kind: 'rateLimited',
        retryAfterMs: retryAfterMs(response.headers.get('Retry-After'), Date.now()),
      };
    }
    if (!response.ok || response.body === null) {
      await response.body?.cancel().catch(() => {});

      return { kind: 'failed' };
    }
    const reader = response.body.getReader();
    for (;;) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }
      arrivals.push({ at: now(), bytes: value.byteLength });
    }

    return { kind: 'measured', bps: probeRate(startedAt, arrivals) };
  } catch {
    // 打ち切りの時間がたったなら、そこまでに届いた量で割る。
    if (timedOut && !options.signal.aborted) {
      return { kind: 'measured', bps: probeRate(startedAt, arrivals) };
    }

    return { kind: 'failed' };
  } finally {
    clearTimeout(timer);
    options.signal.removeEventListener('abort', abort);
  }
}
