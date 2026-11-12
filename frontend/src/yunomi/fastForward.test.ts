import { act, renderHook } from '@testing-library/react';
import { describe, expect, test } from 'vitest';

import { canFastForward, useFastForward } from './fastForward';

describe('canFastForward', () => {
  test.each([
    {
      name: '録り終えた録画は、終わりの手前でも効かせる',
      growing: false,
      position: 595,
      want: true,
    },
    {
      name: '追っかけ再生でも、端から離れていれば効かせる',
      growing: true,
      position: 500,
      want: true,
    },
    {
      name: '追っかけ再生で、端の 10 秒の中では効かせない',
      growing: true,
      position: 591,
      want: false,
    },
    {
      name: '追っかけ再生で、端に追いついたら効かせない',
      growing: true,
      position: 600,
      want: false,
    },
  ])('$name', ({ growing, position, want }) => {
    expect(canFastForward({ growing, position, duration: 600 })).toBe(want);
  });
});

describe('useFastForward', () => {
  test('押している間だけ、選んだ速さの 2 倍にし、離すと戻す', () => {
    const { result } = renderHook(() =>
      useFastForward(1.25, { growing: false, position: 0, duration: 600 }),
    );
    expect(result.current.playbackRate).toBe(1.25);

    act(() => result.current.setHeld(true));
    expect(result.current).toMatchObject({ active: true, playbackRate: 2.5 });

    act(() => result.current.setHeld(false));
    expect(result.current).toMatchObject({ active: false, playbackRate: 1.25 });
  });

  test('押したまま端に近づいたら、元の速さに戻す', () => {
    const { result, rerender } = renderHook(
      ({ position }) => useFastForward(1, { growing: true, position, duration: 600 }),
      { initialProps: { position: 500 } },
    );
    act(() => result.current.setHeld(true));
    expect(result.current.playbackRate).toBe(2);

    rerender({ position: 595 });
    expect(result.current).toMatchObject({ active: false, playbackRate: 1 });
  });
});
