import { useEffect, type RefObject } from 'react';

// dragThreshold は、押したまま動かしたときに、クリックではなくドラッグとみなす距離 (px)。
export const dragThreshold = 5;

type Point = { x: number; y: number };

// dragScrollPosition は、ドラッグを始めたときのスクロールの位置と、ポインターの動きから、スクロールの位置を求める。
// ポインターを右下へ動かすと、中身も右下へ動く (スクロールの位置は左上へ戻る)。
export function dragScrollPosition(scrollStart: Point, pointerStart: Point, pointer: Point): Point {
  return {
    x: scrollStart.x - (pointer.x - pointerStart.x),
    y: scrollStart.y - (pointer.y - pointerStart.y),
  };
}

// useDragScroll は、マウスで押したままドラッグして、要素をスクロールできるようにする (#698)。
// タッチとペンは、ブラウザがもともとスクロールするので、マウスだけを扱う。
// ドラッグした後に届くクリックは捨て、番組の詳細などを開かないようにする。少しだけ動いたときは、クリックのままにする。
export function useDragScroll(ref: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    const element = ref.current;
    if (element === null) {
      return;
    }
    let start: { scroll: Point; pointer: Point } | null = null;
    let dragging = false;

    const onPointerDown = (event: PointerEvent) => {
      if (event.pointerType !== 'mouse' || event.button !== 0) {
        return;
      }
      start = {
        scroll: { x: element.scrollLeft, y: element.scrollTop },
        pointer: { x: event.clientX, y: event.clientY },
      };
      dragging = false;
    };
    const onPointerMove = (event: PointerEvent) => {
      if (start === null) {
        return;
      }
      const pointer = { x: event.clientX, y: event.clientY };
      if (
        !dragging &&
        Math.hypot(pointer.x - start.pointer.x, pointer.y - start.pointer.y) < dragThreshold
      ) {
        return;
      }
      if (!dragging) {
        dragging = true;
        element.style.cursor = 'grabbing';
        element.style.userSelect = 'none';
      }
      const position = dragScrollPosition(start.scroll, start.pointer, pointer);
      element.scrollLeft = position.x;
      element.scrollTop = position.y;
    };
    const onPointerUp = () => {
      if (start === null) {
        return;
      }
      start = null;
      if (!dragging) {
        return;
      }
      element.style.cursor = '';
      element.style.userSelect = '';
      // ドラッグを終えたときのクリックだけを捨てる。クリックが届かないこともあるので、次のタスクで取り下げる。
      const swallow = (click: MouseEvent) => {
        click.preventDefault();
        click.stopPropagation();
      };
      element.addEventListener('click', swallow, { capture: true, once: true });
      setTimeout(() => element.removeEventListener('click', swallow, { capture: true }), 0);
    };
    // リンクなどをブラウザがドラッグアンドドロップとして持ち上げると、ポインターの動きが届かなくなるので止める。
    const onDragStart = (event: DragEvent) => event.preventDefault();

    element.addEventListener('pointerdown', onPointerDown);
    window.addEventListener('pointermove', onPointerMove);
    window.addEventListener('pointerup', onPointerUp);
    window.addEventListener('pointercancel', onPointerUp);
    element.addEventListener('dragstart', onDragStart);

    return () => {
      element.removeEventListener('pointerdown', onPointerDown);
      window.removeEventListener('pointermove', onPointerMove);
      window.removeEventListener('pointerup', onPointerUp);
      window.removeEventListener('pointercancel', onPointerUp);
      element.removeEventListener('dragstart', onDragStart);
    };
  }, [ref]);
}
