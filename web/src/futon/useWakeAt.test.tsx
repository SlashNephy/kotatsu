import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { nextWakeAt, useNow, useWakeAt } from './useWakeAt';

const start = Date.parse('2026-10-19T12:00:00+09:00');

describe('useWakeAt', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(start);
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  test.each([
    { name: '時刻になったら 1 回だけ呼ぶ', at: start + 60_000, before: 59_999, after: 1 },
    { name: '過ぎた時刻なら、1 秒あけて呼ぶ', at: start - 60_000, before: 999, after: 1 },
  ])('$name', ({ at, before, after }) => {
    const onWake = vi.fn();
    renderHook(() => useWakeAt(at, onWake));

    act(() => {
      vi.advanceTimersByTime(before);
    });
    expect(onWake).not.toHaveBeenCalled();
    act(() => {
      vi.advanceTimersByTime(after);
    });
    expect(onWake).toHaveBeenCalledTimes(1);
    act(() => {
      vi.advanceTimersByTime(10 * 60_000);
    });
    expect(onWake).toHaveBeenCalledTimes(1);
  });

  test('null なら呼ばない', () => {
    const onWake = vi.fn();
    renderHook(() => useWakeAt(null, onWake));

    act(() => {
      vi.advanceTimersByTime(24 * 3_600_000);
    });
    expect(onWake).not.toHaveBeenCalled();
  });

  test('setTimeout の上限を超える時刻でも、すぐには呼ばない', () => {
    const onWake = vi.fn();
    const far = 30 * 24 * 3_600_000;
    renderHook(() => useWakeAt(start + far, onWake));

    act(() => {
      vi.advanceTimersByTime(far - 1);
    });
    expect(onWake).not.toHaveBeenCalled();
    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(onWake).toHaveBeenCalledTimes(1);
  });

  test('スリープから戻って画面が見えたとき、時刻を過ぎていれば呼ぶ', () => {
    const onWake = vi.fn();
    renderHook(() => useWakeAt(start + 60_000, onWake));

    // スリープの間はタイマーが進まず、壁時計だけが進んだことにする。
    vi.setSystemTime(start + 5 * 60_000);
    act(() => {
      document.dispatchEvent(new Event('visibilitychange'));
    });
    expect(onWake).toHaveBeenCalledTimes(1);
  });

  test('時刻が変わったら、前の時刻では呼ばず、新しい時刻で呼ぶ', () => {
    const onWake = vi.fn();
    const { rerender } = renderHook(({ at }) => useWakeAt(at, onWake), {
      initialProps: { at: start + 60_000 },
    });

    rerender({ at: start + 120_000 });
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(onWake).not.toHaveBeenCalled();
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(onWake).toHaveBeenCalledTimes(1);
  });
});

describe('useNow', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(start + 30_000);
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  test('区切りごとに進む', () => {
    const { result } = renderHook(() => useNow(60_000));
    expect(result.current).toBe(start + 30_000);

    act(() => {
      vi.advanceTimersByTime(29_999);
    });
    expect(result.current).toBe(start + 30_000);
    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(result.current).toBe(start + 60_000);
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(result.current).toBe(start + 120_000);
  });
});

describe('nextWakeAt', () => {
  const at = (minutes: number) => new Date(start + minutes * 60_000).toISOString();

  test.each([
    {
      name: '後の時刻のうち最も早いもの',
      times: [at(30), at(5), at(10)],
      want: start + 5 * 60_000,
    },
    { name: '今と過ぎた時刻は選ばない', times: [at(0), at(-5), at(20)], want: start + 20 * 60_000 },
    { name: '上限より先なら上限', times: [at(90)], want: start + 60 * 60_000 },
    { name: '時刻がなければ上限', times: [], want: start + 60 * 60_000 },
  ])('$name', ({ times, want }) => {
    expect(nextWakeAt(times, start, 60 * 60_000)).toBe(want);
  });
});
