import { useRef } from 'react';

import { onVideoSurface } from './useControlsVisibility';

// doubleTapInterval と doubleTapDistance は、2 回のタップを、ダブルタップとみなす間隔と距離。
const doubleTapInterval = 300;
const doubleTapDistance = 40;

// TapSide は、ダブルタップした所。映像を左右と真ん中の 3 つに分ける。
export type TapSide = 'left' | 'center' | 'right';

// tapSide は、映像の幅 width のうち、x の位置が、左右と真ん中のどの 3 分の 1 かを返す。
export function tapSide(x: number, width: number): TapSide {
  if (x < width / 3) {
    return 'left';
  }

  return x > (width * 2) / 3 ? 'right' : 'center';
}

// useDoubleTap は、タッチで映像をダブルタップしたら、その場所を onDoubleTap に渡す、pointerup のハンドラーを返す (#452)。
// マウスは、ブラウザの dblclick で扱うので、ここでは見ない。
export function useDoubleTap(
  onDoubleTap: (side: TapSide) => void,
): (event: React.PointerEvent<HTMLElement>) => void {
  const last = useRef<{ time: number; x: number; y: number } | null>(null);

  return (event) => {
    if (event.pointerType !== 'touch' || !onVideoSurface(event.target)) {
      return;
    }
    const prev = last.current;
    const now = event.timeStamp;
    if (
      prev !== null &&
      now - prev.time <= doubleTapInterval &&
      Math.hypot(event.clientX - prev.x, event.clientY - prev.y) <= doubleTapDistance
    ) {
      last.current = null;
      const rect = event.currentTarget.getBoundingClientRect();
      onDoubleTap(tapSide(event.clientX - rect.left, rect.width));

      return;
    }
    last.current = { time: now, x: event.clientX, y: event.clientY };
  };
}
