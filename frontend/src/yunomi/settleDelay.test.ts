import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { createSettler, settleMs, useSettledFalse } from './settleDelay';

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('createSettler', () => {
  test('変化は、落ち着くまで待ってから反映する', () => {
    const apply = vi.fn();
    const settler = createSettler<boolean>(false, apply);

    settler.set(true);
    vi.advanceTimersByTime(settleMs - 1);
    expect(apply).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(apply).toHaveBeenCalledWith(true);
  });

  test('待つ間に元に戻れば、何もしない', () => {
    const apply = vi.fn();
    const settler = createSettler<boolean>(false, apply);

    settler.set(true);
    vi.advanceTimersByTime(100);
    settler.set(false);
    vi.advanceTimersByTime(settleMs * 2);
    expect(apply).not.toHaveBeenCalled();
  });

  test('すぐに反映するときは、待っていた変化を捨てる', () => {
    const apply = vi.fn();
    const settler = createSettler<boolean>(true, apply);

    settler.set(false);
    settler.now(true);
    vi.advanceTimersByTime(settleMs * 2);
    expect(apply.mock.calls).toEqual([[true]]);
  });
});

describe('useSettledFalse', () => {
  test('true はすぐに、false は落ち着いてから返す', () => {
    const { result, rerender } = renderHook(({ value }) => useSettledFalse(value), {
      initialProps: { value: true },
    });
    expect(result.current).toBe(true);

    rerender({ value: false });
    expect(result.current).toBe(true);
    // 落ち着く前にまた true になれば、そのまま出し続ける。
    act(() => vi.advanceTimersByTime(100));
    rerender({ value: true });
    act(() => vi.advanceTimersByTime(settleMs));
    expect(result.current).toBe(true);

    rerender({ value: false });
    act(() => vi.advanceTimersByTime(settleMs));
    expect(result.current).toBe(false);
  });
});
