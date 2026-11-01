import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { useSleepTimer } from './sleepTimer';

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('useSleepTimer', () => {
  test('選んだ時間が経ったら止め、選択を切る', () => {
    const onStop = vi.fn();
    const { result } = renderHook(() => useSleepTimer(onStop));

    act(() => result.current.setChoice('15'));
    expect(result.current.remaining).toBe(15);

    act(() => {
      vi.advanceTimersByTime(5 * 60_000);
    });
    expect(result.current.remaining).toBe(10);
    expect(onStop).not.toHaveBeenCalled();

    act(() => {
      vi.advanceTimersByTime(10 * 60_000);
    });
    expect(onStop).toHaveBeenCalledTimes(1);
    expect(result.current.choice).toBe('off');
    expect(result.current.remaining).toBeNull();
  });

  test('切ったら、時間が経っても止めない', () => {
    const onStop = vi.fn();
    const { result } = renderHook(() => useSleepTimer(onStop));

    act(() => result.current.setChoice('15'));
    act(() => result.current.setChoice('off'));
    act(() => {
      vi.advanceTimersByTime(20 * 60_000);
    });
    expect(onStop).not.toHaveBeenCalled();
  });

  test('番組の終わりを選んだら、時間では止めず、fire で止める', () => {
    const onStop = vi.fn();
    const { result } = renderHook(() => useSleepTimer(onStop));

    act(() => result.current.setChoice('programEnd'));
    act(() => {
      vi.advanceTimersByTime(120 * 60_000);
    });
    expect(onStop).not.toHaveBeenCalled();

    act(() => result.current.fire());
    expect(onStop).toHaveBeenCalledTimes(1);
  });
});
