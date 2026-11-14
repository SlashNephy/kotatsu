import { renderHook } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { swipeStep, swipeThreshold } from './swipe';
import { useSwipeGestures, type SwipeGestureOptions } from './useSwipeGestures';

// box は、幅 1000、高さ 500 のプレイヤーを包む要素。
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
function pointer(
  box: HTMLElement,
  type: string,
  x: number,
  y: number,
  init: { id?: number; pointerType?: string } = {},
) {
  const event = new MouseEvent(type, { bubbles: true, clientX: x, clientY: y });
  Object.assign(event, { pointerId: init.id ?? 1, pointerType: init.pointerType ?? 'touch' });
  box.dispatchEvent(event);
}

function render(box: HTMLElement, options: Partial<SwipeGestureOptions> = {}) {
  const onStep = vi.fn();
  const onSwipeStart = vi.fn();
  renderHook(() =>
    useSwipeGestures(
      { current: box },
      {
        enabled: true,
        controlsVisible: false,
        volumeAvailable: true,
        onStep,
        onSwipeStart,
        ...options,
      },
    ),
  );

  return { onStep, onSwipeStart };
}

afterEach(() => {
  document.body.replaceChildren();
});

describe('useSwipeGestures (#1598)', () => {
  test('右の端を上へなぞると、音量を上げ、押したときに出した操作バーを隠し直す', () => {
    const box = makeBox();
    const { onStep, onSwipeStart } = render(box);

    pointer(box, 'pointerdown', 900, 400);
    pointer(box, 'pointermove', 900, 400 - swipeStep * 2);
    pointer(box, 'pointerup', 900, 400 - swipeStep * 2);

    expect(onSwipeStart).toHaveBeenCalledOnce();
    expect(onStep).toHaveBeenCalledWith('volume', 2);
  });

  test('左の端を下へなぞると、明るさを下げる', () => {
    const box = makeBox();
    const { onStep } = render(box);

    pointer(box, 'pointerdown', 100, 100);
    pointer(box, 'pointermove', 100, 100 + swipeStep);

    expect(onStep).toHaveBeenCalledWith('brightness', -1);
  });

  test.each([
    { name: '操作バーを出している間', options: { controlsVisible: true }, pointerType: 'touch' },
    {
      name: '入れていないとき (全画面でないか、設定で切っている)',
      options: { enabled: false },
      pointerType: 'touch',
    },
    { name: 'マウス', options: {}, pointerType: 'mouse' },
  ])('$name は受け付けない', ({ options, pointerType }) => {
    const box = makeBox();
    const { onStep } = render(box, options);

    pointer(box, 'pointerdown', 900, 400, { pointerType });
    pointer(box, 'pointermove', 900, 400 - swipeStep * 3, { pointerType });

    expect(onStep).not.toHaveBeenCalled();
  });

  test('2 本目の指が触れたら (ピンチ)、やめる', () => {
    const box = makeBox();
    const { onStep } = render(box);

    pointer(box, 'pointerdown', 900, 400, { id: 1 });
    pointer(box, 'pointerdown', 500, 400, { id: 2 });
    pointer(box, 'pointermove', 900, 400 - swipeStep * 3, { id: 1 });

    expect(onStep).not.toHaveBeenCalled();
  });

  test('閾値より短い動きは、タップとして残す', () => {
    const box = makeBox();
    const { onStep, onSwipeStart } = render(box);

    pointer(box, 'pointerdown', 900, 400);
    pointer(box, 'pointermove', 900, 400 - swipeThreshold + 2);
    pointer(box, 'pointerup', 900, 400 - swipeThreshold + 2);

    expect(onSwipeStart).not.toHaveBeenCalled();
    expect(onStep).not.toHaveBeenCalled();
  });
});
