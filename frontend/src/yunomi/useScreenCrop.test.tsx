import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { cropForMode } from './crop';
import { useScreenCrop } from './useScreenCrop';

const initial = { width: window.innerWidth, height: window.innerHeight };

// resizeTo は、画面の大きさを変え、ブラウザと同じく resize を送る。
function resizeTo(width: number, height: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: width });
  Object.defineProperty(window, 'innerHeight', { configurable: true, value: height });
  window.dispatchEvent(new Event('resize'));
}

afterEach(() => {
  resizeTo(initial.width, initial.height);
});

describe('useScreenCrop (#1599)', () => {
  test('全画面で画面いっぱいに広げるときは、画面の大きさが変わったら覆い直す', () => {
    resizeTo(1920, 1080);
    const { result } = renderHook(() => useScreenCrop(cropForMode('fill'), true));
    expect(result.current.scale).toBeCloseTo(1);

    act(() => resizeTo(2400, 1080));
    expect(result.current.scale).toBeCloseTo(2400 / 1080 / (16 / 9));
  });

  test('全画面でなければ、拡大しない', () => {
    resizeTo(2400, 1080);
    const { result } = renderHook(() => useScreenCrop(cropForMode('fill'), false));

    expect(result.current.scale).toBe(1);
  });

  test('ほかの表示の比率は、そのまま返す', () => {
    const crop = cropForMode('pillarbox');
    const { result } = renderHook(() => useScreenCrop(crop, true));

    expect(result.current).toBe(crop);
  });
});
