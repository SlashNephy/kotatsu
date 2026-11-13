import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import {
  liveThresholdSeconds,
  resumeBehindMs,
  useLiveTimeshift,
  type TimeshiftMode,
} from './useLiveTimeshift';
import { loadLiveResume, saveLiveResume } from './liveResume';

const origin = Date.parse('2026-10-29T08:00:00Z');
// buffer は、10 分前から録っているバッファ。途切れた区間はない。
const buffer = {
  from: new Date(origin - 600_000).toISOString(),
  gaps: [],
  streamUrl: '/internal/streams/timeshift/1?from=x',
  hlsUrl: '/internal/streams/timeshift/1-0',
};

function fire(video: HTMLVideoElement, type: string) {
  act(() => {
    video.dispatchEvent(new Event(type));
  });
}

beforeEach(() => {
  localStorage.clear();
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
    const { result } = renderHook(() => useLiveTimeshift(buffer, video, 'convert'));
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
    const { result } = renderHook(() => useLiveTimeshift(buffer, video, 'convert'));
    act(() => {
      result.current.seekTo(result.current.edge - back);
    });

    expect(result.current.behind).toBe(behind);
  });

  test('kyusu の経路では、仮想 HLS を起点からの位置で読み直す', () => {
    const video = document.createElement('video');
    const { result } = renderHook(() => useLiveTimeshift(buffer, video, 'hls'));
    act(() => {
      result.current.seekTo(120);
    });

    expect(result.current.behind).toBe(true);
    expect(result.current.hlsStart).toBeCloseTo(120, 0);
    expect(result.current.hlsLoads).toBe(1);
    expect(result.current.frame?.hlsUrl).toBe(buffer.hlsUrl);
    // 仮想 HLS は、ブラウザの中で変換する配信を使わないので、HEAD を送らない。
    expect(fetch).not.toHaveBeenCalled();
  });

  test('バッファを再生している間に経路を替えたら、新しい経路でそれまでの位置から読み直す', () => {
    const video = document.createElement('video');
    const { result, rerender } = renderHook(({ mode }) => useLiveTimeshift(buffer, video, mode), {
      initialProps: { mode: 'convert' as TimeshiftMode },
    });
    act(() => {
      result.current.seekTo(120);
    });
    expect(result.current.hlsLoads).toBe(0);

    rerender({ mode: 'hls' });
    expect(result.current.behind).toBe(true);
    expect(result.current.hlsLoads).toBe(1);
    expect(result.current.hlsStart).toBeCloseTo(120, 0);
  });

  test('経路を替えてバッファを再生できなくなったら、ライブに戻ったものとして扱う', () => {
    const video = document.createElement('video');
    const { result, rerender } = renderHook(({ mode }) => useLiveTimeshift(buffer, video, mode), {
      initialProps: { mode: 'convert' as TimeshiftMode },
    });
    act(() => {
      result.current.seekTo(60);
    });
    expect(result.current.behind).toBe(true);

    rerender({ mode: null });
    expect(result.current.behind).toBe(false);
    expect(result.current.available).toBe(false);
  });
});

describe('useLiveTimeshift の、見ていた位置の再開 (#1600)', () => {
  const service = 'U2VydmljZTox';

  test('バッファを見ている間は、見ている放送時刻を覚え、ライブに戻ったら忘れる', () => {
    const video = document.createElement('video');
    const { result } = renderHook(() => useLiveTimeshift(buffer, video, 'hls', service));
    act(() => {
      result.current.seekTo(120);
    });
    act(() => {
      vi.advanceTimersByTime(5_000);
    });
    // バッファの始まりから 120 秒の放送時刻。
    expect(loadLiveResume(localStorage, service)).toBe(origin - 600_000 + 120_000);

    act(() => {
      result.current.goLive();
    });
    expect(loadLiveResume(localStorage, service)).toBeNull();
  });

  test('ライブの端を見ている間は、覚えない', () => {
    const video = document.createElement('video');
    renderHook(() => useLiveTimeshift(buffer, video, 'hls', service));
    act(() => {
      vi.advanceTimersByTime(30_000);
    });

    expect(loadLiveResume(localStorage, service)).toBeNull();
  });

  test.each([
    { name: 'バッファに残っていれば、その位置から再開する', ago: 300_000, behind: true },
    // バッファの始まりより前は、もう残っていない。
    { name: 'バッファより前なら、ライブの端から始めて忘れる', ago: 900_000, behind: false },
    {
      name: 'ライブの端に近ければ、ライブの端から始めて忘れる',
      ago: (liveThresholdSeconds - 1) * 1000,
      behind: false,
    },
  ])('$name', ({ ago, behind }) => {
    saveLiveResume(localStorage, service, origin - ago, origin - 1_000);
    const video = document.createElement('video');
    const { result } = renderHook(() => useLiveTimeshift(buffer, video, 'hls', service));

    expect(result.current.behind).toBe(behind);
    if (behind) {
      expect(result.current.hlsStart).toBeCloseTo((600_000 - ago) / 1000, 0);
      // 移れたら、続けて見ている位置を覚え直す。
      act(() => {
        vi.advanceTimersByTime(5_000);
      });
      expect(loadLiveResume(localStorage, service)).toBe(origin - ago);
    } else {
      expect(loadLiveResume(localStorage, service)).toBeNull();
    }
  });

  test('ほかのサービスで覚えた位置では、再開しない', () => {
    saveLiveResume(localStorage, 'other', origin - 300_000, origin);
    const video = document.createElement('video');
    const { result } = renderHook(() => useLiveTimeshift(buffer, video, 'hls', service));

    expect(result.current.behind).toBe(false);
    expect(loadLiveResume(localStorage, 'other')).toBe(origin - 300_000);
  });
});
