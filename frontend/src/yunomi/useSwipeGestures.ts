import { useEffect, useRef, type RefObject } from 'react';

import { swipeBandAt, SwipeTracker, type SwipeBand } from './swipe';
import { onVideoSurface } from './useControlsVisibility';

export type SwipeGestureOptions = {
  // enabled は、スワイプを受け付けるか。端末の設定で入れていて、全画面のときだけ受け付ける。
  enabled: boolean;
  // controlsVisible は、操作バーを出しているか。出している間は、スワイプを受け付けない。
  controlsVisible: boolean;
  // volumeAvailable は、音量の帯を出すか。iOS では、ページから音量を変えられない。
  volumeAvailable: boolean;
  // onStep は、帯 band を steps 段変える。上へ動かすと正。
  onStep: (band: SwipeBand, steps: number) => void;
  // onSwipeStart は、縦のスワイプと決めたときに呼ぶ。押したときに出した操作バーを隠し直すのに使う。
  onSwipeStart: () => void;
};

// useSwipeGestures は、プレイヤーを包む要素 box で、映像の左右の端を縦にスワイプして、明るさと音量を変える (#1598)。
// タッチの 1 本の指だけを追い、2 本目の指が触れたら (ピンチ) やめる。マウスでは受け付けない。
// React の pointerdown より先に、押した時点で操作バーを出していたかを読むため、要素に直に聞き手を付ける。
export function useSwipeGestures(
  box: RefObject<HTMLElement | null>,
  options: SwipeGestureOptions,
): void {
  const latest = useRef(options);
  latest.current = options;

  useEffect(() => {
    const element = box.current;
    if (element === null || !options.enabled) {
      return undefined;
    }
    let tracker: SwipeTracker | null = null;
    let pointerId: number | null = null;
    const touches = new Set<number>();

    const onPointerDown = (event: PointerEvent) => {
      if (event.pointerType !== 'touch') {
        return;
      }
      touches.add(event.pointerId);
      // 2 本目の指が触れたら、スワイプとして扱わない。
      if (touches.size > 1) {
        tracker = null;

        return;
      }
      const { controlsVisible, volumeAvailable } = latest.current;
      if (controlsVisible || !onVideoSurface(event.target)) {
        return;
      }
      const rect = element.getBoundingClientRect();
      const band = swipeBandAt(event.clientX - rect.left, rect.width, volumeAvailable);
      if (band !== null) {
        tracker = new SwipeTracker(band, event.clientX, event.clientY);
        pointerId = event.pointerId;
      }
    };
    const onPointerMove = (event: PointerEvent) => {
      if (tracker === null || event.pointerId !== pointerId) {
        return;
      }
      const wasPending = tracker.direction === 'pending';
      const steps = tracker.move(event.clientX, event.clientY);
      if (wasPending && tracker.direction === 'vertical') {
        latest.current.onSwipeStart();
      }
      if (steps !== 0) {
        latest.current.onStep(tracker.band, steps);
      }
    };
    const onPointerEnd = (event: PointerEvent) => {
      touches.delete(event.pointerId);
      if (event.pointerId === pointerId) {
        tracker = null;
        pointerId = null;
      }
    };

    element.addEventListener('pointerdown', onPointerDown);
    element.addEventListener('pointermove', onPointerMove);
    element.addEventListener('pointerup', onPointerEnd);
    element.addEventListener('pointercancel', onPointerEnd);
    // 縦に指を動かしても、ブラウザがスクロールや拡大に取らないようにする。
    const touchAction = element.style.touchAction;
    element.style.touchAction = 'none';

    return () => {
      element.removeEventListener('pointerdown', onPointerDown);
      element.removeEventListener('pointermove', onPointerMove);
      element.removeEventListener('pointerup', onPointerEnd);
      element.removeEventListener('pointercancel', onPointerEnd);
      element.style.touchAction = touchAction;
    };
  }, [box, options.enabled]);
}
