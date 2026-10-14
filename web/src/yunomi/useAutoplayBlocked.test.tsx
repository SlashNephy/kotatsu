import { act, renderHook } from '@testing-library/react';
import { describe, expect, test, vi } from 'vitest';

import { useAutoplayBlocked } from './useAutoplayBlocked';

// fakeVideo は、play() の結果を差し替えられる video の要素。
function fakeVideo(play: () => Promise<void>, { paused = true, readyState = 0 } = {}) {
  const video = document.createElement('video');
  Object.defineProperty(video, 'paused', { configurable: true, get: () => paused });
  Object.defineProperty(video, 'readyState', { configurable: true, get: () => readyState });
  const playMock = vi.fn(play);
  video.play = playMock;

  return {
    video,
    play: playMock,
    setPaused: (next: boolean) => {
      paused = next;
    },
    setReadyState: (next: number) => {
      readyState = next;
    },
  };
}

const notAllowed = () => Promise.reject(new DOMException('blocked', 'NotAllowedError'));

describe('useAutoplayBlocked', () => {
  test('再生できるようになっても、ブラウザに再生を断られたら、断られたと返す', async () => {
    const { video, setReadyState } = fakeVideo(notAllowed);
    const { result } = renderHook(() => useAutoplayBlocked(video));
    expect(result.current.blocked).toBe(false);

    setReadyState(3);
    await act(async () => {
      video.dispatchEvent(new Event('canplay'));
    });
    expect(result.current.blocked).toBe(true);
  });

  test('開いたときにすでに再生できる状態なら、すぐに確かめる', async () => {
    const { video } = fakeVideo(notAllowed, { readyState: 4 });
    const { result } = renderHook(() => useAutoplayBlocked(video));
    await act(async () => {});
    expect(result.current.blocked).toBe(true);
  });

  test('ほかの理由で再生に失敗したときは、断られたとしない', async () => {
    const { video } = fakeVideo(() => Promise.reject(new DOMException('aborted', 'AbortError')), {
      readyState: 4,
    });
    const { result } = renderHook(() => useAutoplayBlocked(video));
    await act(async () => {});
    expect(result.current.blocked).toBe(false);
  });

  test('再生が始まったら、断られた状態を解き、その後は再生を試さない', async () => {
    const { video, play, setPaused } = fakeVideo(notAllowed, { readyState: 4 });
    const { result } = renderHook(() => useAutoplayBlocked(video));
    await act(async () => {});
    expect(result.current.blocked).toBe(true);

    setPaused(false);
    act(() => {
      video.dispatchEvent(new Event('play'));
    });
    expect(result.current.blocked).toBe(false);

    // 一度再生したあとに止めてシークしても、勝手に再生し直さない。
    setPaused(true);
    play.mockClear();
    await act(async () => {
      video.dispatchEvent(new Event('canplay'));
    });
    expect(play).not.toHaveBeenCalled();
  });

  test('play で、ボタンから再生を始める', async () => {
    const { video, play, setPaused } = fakeVideo(notAllowed, { readyState: 4 });
    const { result } = renderHook(() => useAutoplayBlocked(video));
    await act(async () => {});

    play.mockImplementation(() => {
      setPaused(false);
      video.dispatchEvent(new Event('play'));

      return Promise.resolve();
    });
    await act(async () => {
      result.current.play();
    });
    expect(result.current.blocked).toBe(false);
  });
});
