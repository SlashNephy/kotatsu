import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { slideSeekHoldMs, slideSeekSecondsPerWidth, slideSeekThreshold } from './slideSeek';
import { useSlideSeek, type SlideSeekOptions } from './useSlideSeek';

function makeBox() {
  const box = document.createElement('div');
  box.getBoundingClientRect = () => ({
    left: 0,
    top: 0,
    width: 1000,
    height: 500,
    right: 1000,
    bottom: 500,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  });
  document.body.append(box);

  return box;
}

// pointer は、box にポインターのイベントを送る。jsdom には PointerEvent がないので、MouseEvent に足す。
function pointer(box: HTMLElement, type: string, x: number, y = 250, pointerType = 'touch') {
  const event = new MouseEvent(type, { bubbles: true, cancelable: true, clientX: x, clientY: y });
  Object.assign(event, { pointerId: 1, pointerType, isPrimary: true });
  box.dispatchEvent(event);

  return event;
}

function render(box: HTMLElement, options: Partial<SlideSeekOptions> = {}) {
  const handlers = {
    onSlideStart: vi.fn(),
    onScrub: vi.fn(),
    onCommit: vi.fn(),
    onCancel: vi.fn(),
    onLongPress: vi.fn(),
  };
  // 要素の参照は、描き直しても同じものを渡す (画面では useRef の参照を渡す)。
  const ref = { current: box };
  const view = renderHook(() =>
    useSlideSeek(ref, {
      enabled: true,
      range: () => ({ position: 600, duration: 3600 }),
      ...handlers,
      ...options,
    }),
  );

  return { ...handlers, view };
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  document.body.replaceChildren();
});

describe('useSlideSeek (#1601)', () => {
  test('長押しのまま横に滑らせると、行き先を出し、離したらそこへシークする', () => {
    const box = makeBox();
    const { onSlideStart, onScrub, onCommit, onLongPress, view } = render(box);

    pointer(box, 'pointerdown', 500);
    act(() => {
      vi.advanceTimersByTime(slideSeekHoldMs);
    });
    act(() => {
      pointer(box, 'pointermove', 500 + slideSeekThreshold);
      pointer(box, 'pointermove', 500 + slideSeekThreshold + 500);
    });
    expect(onSlideStart).toHaveBeenCalledOnce();
    expect(view.result.current).toBeCloseTo(600 + slideSeekSecondsPerWidth / 2);
    act(() => {
      pointer(box, 'pointerup', 500 + slideSeekThreshold + 500);
    });

    expect(onScrub).toHaveBeenLastCalledWith(600 + slideSeekSecondsPerWidth / 2);
    expect(onCommit).toHaveBeenCalledWith(600 + slideSeekSecondsPerWidth / 2);
    expect(onLongPress).not.toHaveBeenCalled();
    expect(view.result.current).toBeNull();
  });

  test('滑らせずに離したら、押した位置で長押しのメニューを開き、ブラウザのメニューは止める', () => {
    const box = makeBox();
    const { onCommit, onLongPress } = render(box);

    pointer(box, 'pointerdown', 300, 200);
    act(() => {
      vi.advanceTimersByTime(slideSeekHoldMs);
    });
    const contextmenu = pointer(box, 'contextmenu', 300, 200);
    expect(contextmenu.defaultPrevented).toBe(true);
    act(() => {
      pointer(box, 'pointerup', 300, 200);
    });

    expect(onLongPress).toHaveBeenCalledWith(300, 200);
    expect(onCommit).not.toHaveBeenCalled();
    // 離した後に届くマウスの互換のイベントは、開いたメニューを閉じないよう止める。
    const mousedown = new MouseEvent('mousedown', { bubbles: true, cancelable: true });
    document.body.dispatchEvent(mousedown);
    expect(mousedown.defaultPrevented).toBe(true);
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    const later = new MouseEvent('mousedown', { bubbles: true, cancelable: true });
    document.body.dispatchEvent(later);
    expect(later.defaultPrevented).toBe(false);
  });

  test('滑らせている間に取り消されたら (着信など)、元に戻す', () => {
    const box = makeBox();
    const { onCancel, onCommit } = render(box);

    pointer(box, 'pointerdown', 500);
    act(() => {
      vi.advanceTimersByTime(slideSeekHoldMs);
    });
    act(() => {
      pointer(box, 'pointermove', 500 + slideSeekThreshold + 100);
    });
    act(() => {
      pointer(box, 'pointercancel', 500 + slideSeekThreshold + 100);
    });

    expect(onCancel).toHaveBeenCalledOnce();
    expect(onCommit).not.toHaveBeenCalled();
  });

  test('長押しになる前に動かしたら (スワイプ)、何もしない', () => {
    const box = makeBox();
    const { onScrub, onLongPress } = render(box);

    pointer(box, 'pointerdown', 500);
    pointer(box, 'pointermove', 500, 250 - 30);
    act(() => {
      vi.advanceTimersByTime(slideSeekHoldMs);
    });
    act(() => {
      pointer(box, 'pointermove', 800, 220);
    });
    act(() => {
      pointer(box, 'pointerup', 800, 220);
    });

    expect(onScrub).not.toHaveBeenCalled();
    expect(onLongPress).not.toHaveBeenCalled();
  });

  test.each([
    { name: 'マウス', options: {}, pointerType: 'mouse' },
    {
      name: 'シークできないとき (バッファのないライブ視聴)',
      options: { enabled: false },
      pointerType: 'touch',
    },
  ])('$name では受け付けない', ({ options, pointerType }) => {
    const box = makeBox();
    const { onScrub, onLongPress } = render(box, options);

    pointer(box, 'pointerdown', 500, 250, pointerType);
    act(() => {
      vi.advanceTimersByTime(slideSeekHoldMs);
    });
    act(() => {
      pointer(box, 'pointermove', 800, 250, pointerType);
    });
    act(() => {
      pointer(box, 'pointerup', 800, 250, pointerType);
    });

    expect(onScrub).not.toHaveBeenCalled();
    expect(onLongPress).not.toHaveBeenCalled();
  });
});
