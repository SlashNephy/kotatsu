import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { useVideoLoading, waitingDelayMs } from './useVideoLoading';

// fakeVideo は、paused を差し替えられる video の要素。
function fakeVideo() {
  const video = document.createElement('video');
  let paused = true;
  Object.defineProperty(video, 'paused', { configurable: true, get: () => paused });

  return {
    video,
    setPaused: (next: boolean) => {
      paused = next;
    },
    fire: (type: string) => {
      act(() => {
        video.dispatchEvent(new Event(type));
      });
    },
  };
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('useVideoLoading', () => {
  test('読み込みを始めてから、映像が動き出すまで出す', () => {
    const { video, setPaused, fire } = fakeVideo();
    const { result } = renderHook(() => useVideoLoading(video));
    expect(result.current).toBe(true);

    setPaused(false);
    fire('playing');
    expect(result.current).toBe(false);

    // 録画のシークや画質の切り替えで、読み込み直したら、また出す。
    fire('loadstart');
    expect(result.current).toBe(true);
    fire('playing');
    expect(result.current).toBe(false);
  });

  test('再生の途中の待ちは、少し続いたときだけ出す', () => {
    const { video, setPaused, fire } = fakeVideo();
    const { result } = renderHook(() => useVideoLoading(video));
    setPaused(false);
    fire('playing');

    fire('waiting');
    act(() => {
      vi.advanceTimersByTime(waitingDelayMs - 100);
    });
    fire('playing');
    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(result.current).toBe(false);

    fire('waiting');
    act(() => {
      vi.advanceTimersByTime(waitingDelayMs + 100);
    });
    expect(result.current).toBe(true);
  });

  test('止めたままシークしたら、シークし終えたときに消す', () => {
    const { video, setPaused, fire } = fakeVideo();
    const { result } = renderHook(() => useVideoLoading(video));
    setPaused(false);
    fire('playing');

    setPaused(true);
    fire('pause');
    fire('seeking');
    expect(result.current).toBe(true);
    fire('seeked');
    expect(result.current).toBe(false);
  });

  test('止めたまま読み込み終えたら消す (自動再生を断られたときなど)', () => {
    const { video, fire } = fakeVideo();
    const { result } = renderHook(() => useVideoLoading(video));
    expect(result.current).toBe(true);
    fire('canplay');
    expect(result.current).toBe(false);
  });

  test('読み込みに失敗したら消す', () => {
    const { video, fire } = fakeVideo();
    const { result } = renderHook(() => useVideoLoading(video));
    fire('error');
    expect(result.current).toBe(false);
  });

  test('読み込み終えて止まっている要素では、出さない', () => {
    const { video } = fakeVideo();
    Object.defineProperty(video, 'readyState', { configurable: true, get: () => 4 });
    const { result } = renderHook(() => useVideoLoading(video));
    expect(result.current).toBe(false);
  });
});
