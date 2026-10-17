import { renderHook } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { dragScrollPosition, useDragScroll } from './useDragScroll';

describe('dragScrollPosition', () => {
  test('ポインターを右下へ動かすと、スクロールの位置は左上へ戻る', () => {
    expect(dragScrollPosition({ x: 300, y: 500 }, { x: 100, y: 100 }, { x: 140, y: 160 })).toEqual({
      x: 260,
      y: 440,
    });
  });
});

describe('useDragScroll', () => {
  let element: HTMLDivElement;
  afterEach(() => {
    element.remove();
    vi.useRealTimers();
  });

  const pointer = (
    type: string,
    target: EventTarget,
    init: { x: number; y: number; pointerType?: string; button?: number; buttons?: number },
  ) => {
    const event = new MouseEvent(type, {
      bubbles: true,
      cancelable: true,
      clientX: init.x,
      clientY: init.y,
      button: init.button ?? 0,
    });
    Object.defineProperty(event, 'pointerType', { value: init.pointerType ?? 'mouse' });
    // 押しているボタン。離したとき (pointerup) のほかは、主ボタンを押しているとする。
    Object.defineProperty(event, 'buttons', {
      value: init.buttons ?? (type === 'pointerup' ? 0 : 1),
    });
    target.dispatchEvent(event);
  };

  const setup = () => {
    element = document.createElement('div');
    const child = document.createElement('button');
    element.append(child);
    document.body.append(element);
    element.scrollLeft = 0;
    element.scrollTop = 0;
    const onClick = vi.fn();
    child.addEventListener('click', onClick);
    renderHook(() => useDragScroll({ current: element }));

    return { child, onClick };
  };

  test('マウスでドラッグするとスクロールし、ドラッグの後のクリックは捨てる', () => {
    vi.useFakeTimers();
    const { child, onClick } = setup();
    // jsdom は大きさを持たないので、書いた値をそのまま読めるようにする。
    let left = 400;
    let top = 600;
    Object.defineProperty(element, 'scrollLeft', {
      get: () => left,
      set: (v: number) => (left = v),
    });
    Object.defineProperty(element, 'scrollTop', { get: () => top, set: (v: number) => (top = v) });

    pointer('pointerdown', child, { x: 100, y: 100 });
    pointer('pointermove', window, { x: 70, y: 40 });
    pointer('pointerup', window, { x: 70, y: 40 });
    child.click();

    expect([left, top]).toEqual([430, 660]);
    expect(onClick).not.toHaveBeenCalled();
    // 次のクリックは届く。
    vi.runAllTimers();
    child.click();
    expect(onClick).toHaveBeenCalledOnce();
  });

  test('少しだけ動いたときと、タッチのときは、スクロールせずにクリックのままにする', () => {
    const { child, onClick } = setup();
    pointer('pointerdown', child, { x: 100, y: 100 });
    pointer('pointermove', window, { x: 102, y: 103 });
    pointer('pointerup', window, { x: 102, y: 103 });
    child.click();
    pointer('pointerdown', child, { x: 100, y: 100, pointerType: 'touch' });
    pointer('pointermove', window, { x: 300, y: 300, pointerType: 'touch' });
    pointer('pointerup', window, { x: 300, y: 300, pointerType: 'touch' });
    child.click();

    expect([element.scrollLeft, element.scrollTop]).toEqual([0, 0]);
    expect(onClick).toHaveBeenCalledTimes(2);
  });

  test('ウィンドウの外でボタンを離して戻ってきたら、ドラッグを終える', () => {
    setup();
    let left = 0;
    Object.defineProperty(element, 'scrollLeft', {
      get: () => left,
      set: (v: number) => (left = v),
    });

    pointer('pointerdown', element, { x: 100, y: 100 });
    pointer('pointermove', window, { x: 50, y: 100 });
    // ボタンを押していない動きは、ドラッグを終えたことにして、スクロールしない。
    pointer('pointermove', window, { x: 0, y: 100, buttons: 0 });
    pointer('pointermove', window, { x: -100, y: 100, buttons: 1 });

    expect(left).toBe(50);
    expect(element.style.cursor).toBe('');
  });

  test('ドラッグの途中でほかのボタンを離しても、ドラッグを続ける', () => {
    setup();
    let left = 0;
    Object.defineProperty(element, 'scrollLeft', {
      get: () => left,
      set: (v: number) => (left = v),
    });

    pointer('pointerdown', element, { x: 100, y: 100 });
    pointer('pointermove', window, { x: 50, y: 100 });
    pointer('pointerup', window, { x: 50, y: 100, button: 2, buttons: 1 });
    pointer('pointermove', window, { x: 0, y: 100 });

    expect(left).toBe(100);
  });
});
