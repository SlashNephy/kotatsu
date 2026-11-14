import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { saveFullscreenLandscape } from './landscapeLock';
import { useFullscreen } from './useFullscreen';

const lock = vi.fn((_orientation: string) => Promise.resolve());
const unlock = vi.fn();
let fullscreenElement: Element | null = null;

// setFullscreen は、全画面の要素を変え、ブラウザと同じく fullscreenchange を送る。
function setFullscreen(element: Element | null) {
  fullscreenElement = element;
  document.dispatchEvent(new Event('fullscreenchange'));
}

beforeEach(() => {
  vi.stubGlobal('screen', { orientation: { lock, unlock } });
  Object.defineProperty(document, 'fullscreenElement', {
    configurable: true,
    get: () => fullscreenElement,
  });
  localStorage.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  fullscreenElement = null;
});

// renderFullscreen は、全画面にできる要素を包んだ useFullscreen を描く。
function renderFullscreen() {
  const box = document.createElement('div');
  box.requestFullscreen = vi.fn(() => {
    setFullscreen(box);

    return Promise.resolve();
  });

  return renderHook(() => useFullscreen({ current: box }));
}

describe('useFullscreen の横向きの固定 (#1599)', () => {
  test('全画面にしたら横向きに固定し、抜けたら外す', async () => {
    const { result } = renderFullscreen();

    await act(async () => result.current.toggle());
    expect(result.current.active).toBe(true);
    expect(lock).toHaveBeenCalledWith('landscape');

    act(() => setFullscreen(null));
    expect(result.current.active).toBe(false);
    expect(unlock).toHaveBeenCalledOnce();
  });

  test('設定で切っていれば、固定しない', async () => {
    saveFullscreenLandscape(localStorage, false);
    const { result } = renderFullscreen();

    await act(async () => result.current.toggle());
    expect(result.current.active).toBe(true);
    expect(lock).not.toHaveBeenCalled();
  });

  test('全画面のまま画面を離れても、固定を外す', async () => {
    const { result, unmount } = renderFullscreen();
    await act(async () => result.current.toggle());

    unmount();
    expect(unlock).toHaveBeenCalledOnce();
  });
});
