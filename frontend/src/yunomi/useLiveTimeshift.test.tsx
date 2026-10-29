import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { liveThresholdSeconds, resumeBehindMs, useLiveTimeshift } from './useLiveTimeshift';

const origin = Date.parse('2026-10-29T08:00:00Z');
// buffer は、10 分前から録っているバッファ。途切れた区間はない。
const buffer = {
  from: new Date(origin - 600_000).toISOString(),
  gaps: [],
  streamUrl: '/streams/timeshift/1?from=x',
};

function fire(video: HTMLVideoElement, type: string) {
  act(() => {
    video.dispatchEvent(new Event(type));
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(origin);
  // 配信の HEAD は、流し始めた位置を返さない。位置は、指定した位置のままにする。
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(new Response(null, { status: 404 }))),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('useLiveTimeshift', () => {
  test.each([
    { name: 'すぐに再開したら、ライブのまま', pausedMs: resumeBehindMs - 1000, behind: false },
    // ライブの端に近くても、止めた位置から続ける。
    {
      name: '3 秒より長く止めたら、止めた位置からバッファで続ける',
      pausedMs: resumeBehindMs + 1000,
      behind: true,
    },
    { name: '長く止めたら、止めた位置からバッファで続ける', pausedMs: 60_000, behind: true },
  ])('$name', ({ pausedMs, behind }) => {
    const video = document.createElement('video');
    const { result } = renderHook(() => useLiveTimeshift(buffer, video, true));
    fire(video, 'pause');
    act(() => {
      vi.advanceTimersByTime(pausedMs);
    });
    fire(video, 'play');

    expect(result.current.behind).toBe(behind);
    if (behind) {
      // 止めた時刻は、バッファの始まりから 600 秒。
      expect(result.current.position).toBeCloseTo(600, 0);
    }
  });

  test.each([
    { name: 'ライブの端から離れた位置へは、バッファへ移る', back: 60, behind: true },
    {
      name: 'ライブの端に近い位置へは、ライブのまま',
      back: liveThresholdSeconds - 1,
      behind: false,
    },
  ])('$name', ({ back, behind }) => {
    const video = document.createElement('video');
    const { result } = renderHook(() => useLiveTimeshift(buffer, video, true));
    act(() => {
      result.current.seekTo(result.current.edge - back);
    });

    expect(result.current.behind).toBe(behind);
  });

  test('経路を替えてバッファを再生できなくなったら、ライブに戻ったものとして扱う', () => {
    const video = document.createElement('video');
    const { result, rerender } = renderHook(
      ({ enabled }) => useLiveTimeshift(buffer, video, enabled),
      { initialProps: { enabled: true } },
    );
    act(() => {
      result.current.seekTo(60);
    });
    expect(result.current.behind).toBe(true);

    rerender({ enabled: false });
    expect(result.current.behind).toBe(false);
    expect(result.current.available).toBe(false);
  });
});
