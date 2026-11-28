import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import type { PlayerMedia } from './playerMedia';
import { useCmSkip, type CmSkipOptions } from './useCmSkip';

// renderCmSkip は、100〜130 秒を CM とした再生元で、今の位置を position にして useCmSkip を描く。
function renderCmSkip(position: number, options: Partial<CmSkipOptions> = {}) {
  const jumpTo = vi.fn((target: number) => target);
  const hook = renderHook(() =>
    useCmSkip({
      sections: [{ from: 100, to: 130 }],
      media: null,
      mediaRef: { current: null as PlayerMedia | null },
      framePosition: () => null,
      jumpTo,
      currentPosition: () => position,
      scrubbing: false,
      showFlash: vi.fn(),
      opening: { at: 0, skipped: false, resumed: false },
      initialSeek: null,
      ...options,
    }),
  );

  return { jumpTo, ...hook };
}

describe('useCmSkip', () => {
  beforeEach(() => {
    localStorage.setItem('kotatsu.player.cmAutoSkip', 'true');
  });
  afterEach(() => {
    localStorage.clear();
  });

  test.each([
    { name: '進む向きで行き先が CM の中なら、CM を飛ぶ', position: 95, seconds: 10, skipped: true },
    {
      name: '戻る向きは、行き先が CM の中でも飛ばずに移る',
      position: 140,
      seconds: -15,
      skipped: false,
    },
    { name: '行き先が CM の外なら、そのまま移る', position: 50, seconds: 10, skipped: false },
  ])('$name', ({ position, seconds, skipped }) => {
    const { result, jumpTo } = renderCmSkip(position);

    let got = false;
    act(() => {
      got = result.current.skip(seconds);
    });

    expect(got).toBe(skipped);
    // 飛んだときは、行き先ではなく、続いた CM の終わりへ移る。
    expect(jumpTo).toHaveBeenLastCalledWith(skipped ? 130 : position + seconds);
  });

  test('自分でシークして見ていた CM の中で進んでも、飛ばない', () => {
    const { result } = renderCmSkip(110);
    act(() => result.current.seek(110));

    let got = true;
    act(() => {
      got = result.current.skip(10);
    });

    expect(got).toBe(false);
  });

  test('URL で CM の中の位置を渡されて開いたら、その CM は飛ばない', () => {
    const { result } = renderCmSkip(105, { initialSeek: 105 });

    let got = true;
    act(() => {
      got = result.current.skip(10);
    });

    expect(got).toBe(false);
  });

  test('自動で飛ばす設定を切っていれば、CM の中へもそのまま移る', () => {
    localStorage.setItem('kotatsu.player.cmAutoSkip', 'false');
    const { result } = renderCmSkip(95);

    let got = true;
    act(() => {
      got = result.current.skip(10);
    });

    expect(got).toBe(false);
  });
});
