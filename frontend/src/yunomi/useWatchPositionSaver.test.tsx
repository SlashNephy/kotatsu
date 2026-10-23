import { renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { useWatchPositionSaver } from './useWatchPositionSaver';

// saved は、偽の mutation に渡った再生位置 (ミリ秒)。
const saved: number[] = [];

vi.mock('react-relay', () => ({
  graphql: () => ({}),
  useMutation: () => [
    ({ variables }: { variables: { positionMs: number } }) => {
      saved.push(variables.positionMs);
    },
    false,
  ],
}));

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  saved.length = 0;
  vi.useRealTimers();
});

describe('useWatchPositionSaver', () => {
  test('開いた直後は保存せず、再生中はしばらくごとに、止めたらその位置を保存する', () => {
    let position = 0;
    const { rerender } = renderHook(
      ({ playing }) => useWatchPositionSaver('rec', () => position, playing),
      {
        initialProps: { playing: false },
      },
    );
    // 開いた直後は、まだ位置が決まっていないので保存しない。
    expect(saved).toEqual([]);

    rerender({ playing: true });
    position = 20;
    vi.advanceTimersByTime(15_000);
    expect(saved).toEqual([20_000]);

    // 1 秒も動いていなければ、保存し直さない。
    position = 20.4;
    rerender({ playing: false });
    expect(saved).toEqual([20_000]);

    rerender({ playing: true });
    position = 33;
    rerender({ playing: false });
    expect(saved).toEqual([20_000, 33_000]);
  });
});
