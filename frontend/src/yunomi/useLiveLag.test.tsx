import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { liveReloadAfterMs, useLiveLag } from './useLiveLag';

function fire(video: HTMLVideoElement, type: string) {
  act(() => {
    video.dispatchEvent(new Event(type));
  });
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('useLiveLag', () => {
  test.each([
    {
      name: 'すぐに再開したら、読み直さない',
      pausedMs: liveReloadAfterMs - 1000,
      reloadOnResume: true,
      reloads: 0,
    },
    {
      name: '長く止めていたら、再開したときに読み直す',
      pausedMs: liveReloadAfterMs + 1000,
      reloadOnResume: true,
      reloads: 1,
    },
    {
      name: '読み直さない経路では、長く止めても読み直さない',
      pausedMs: liveReloadAfterMs + 1000,
      reloadOnResume: false,
      reloads: 0,
    },
  ])('$name', ({ pausedMs, reloadOnResume, reloads }) => {
    const video = document.createElement('video');
    const onReload = vi.fn();
    const { result } = renderHook(() => useLiveLag(video, { reloadOnResume, onReload }));
    expect(result.current).toBe(false);

    fire(video, 'pause');
    expect(result.current).toBe(true);
    act(() => {
      vi.advanceTimersByTime(pausedMs);
    });
    fire(video, 'play');
    expect(onReload).toHaveBeenCalledTimes(reloads);
    // 読み直すまでは、遅れたまま。
    expect(result.current).toBe(true);

    fire(video, 'loadstart');
    expect(result.current).toBe(false);
  });

  test('読み直した後に再開しても、前の一時停止で読み直さない', () => {
    const video = document.createElement('video');
    const onReload = vi.fn();
    renderHook(() => useLiveLag(video, { reloadOnResume: true, onReload }));

    fire(video, 'pause');
    act(() => {
      vi.advanceTimersByTime(liveReloadAfterMs + 1000);
    });
    fire(video, 'loadstart');
    fire(video, 'play');
    expect(onReload).not.toHaveBeenCalled();
  });

  test('ストリームが終わって止まったのは、遅れとみなさない', () => {
    const video = document.createElement('video');
    Object.defineProperty(video, 'ended', { configurable: true, get: () => true });
    const { result } = renderHook(() =>
      useLiveLag(video, { reloadOnResume: true, onReload: vi.fn() }),
    );

    fire(video, 'pause');
    expect(result.current).toBe(false);
  });
});
