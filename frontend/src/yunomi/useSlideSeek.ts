import { useEffect, useRef, useState, type RefObject } from 'react';

import { SlideSeekTracker, slideSeekHoldMs, slideSeekMoveLimit } from './slideSeek';

// longPressMenuDelayMs は、滑らせずに離してから、長押しのメニューを開くまでの時間。指を離した後に届く、
// マウスの互換のイベントを待つ。
const longPressMenuDelayMs = 250;
import { onVideoSurface } from './useControlsVisibility';

export type SlideSeekOptions = {
  // enabled は、滑らせてシークできるか。ライブ視聴では、バッファへシークできるときだけ。
  enabled: boolean;
  // range は、押した時点の、今の再生位置と、動かせる長さ (秒)。
  range: () => { position: number; duration: number } | null;
  // onSlideStart は、滑らせ始めたときに呼ぶ。押したときに出した操作バーを隠し直すのに使う。
  onSlideStart: () => void;
  // onScrub は、滑らせている間の行き先を渡す。シークバーのつまみを動かしている間と同じく扱う。
  onScrub: (seconds: number) => void;
  // onCommit は、指を離したときに、行き先へシークする。onCancel は、取り消されたときに元に戻す。
  onCommit: (seconds: number) => void;
  onCancel: () => void;
  // onLongPress は、滑らせずに離したときに、押した位置 (画面の座標) で、長押しのメニューを開く。
  onLongPress: (x: number, y: number) => void;
};

// useSlideSeek は、プレイヤーを包む要素 box で、映像を長押ししたまま横に滑らせて、再生位置を動かす (#1601)。
// 長押しのメニュー (Android) は、ブラウザが長押しで出す contextmenu を止め、滑らせずに離した少し後に開く。
// 滑らせている間の行き先を返す。滑らせていなければ null。
export function useSlideSeek(
  box: RefObject<HTMLElement | null>,
  options: SlideSeekOptions,
): number | null {
  const latest = useRef(options);
  latest.current = options;
  const [scrub, setScrub] = useState<number | null>(null);

  useEffect(() => {
    const element = box.current;
    if (element === null || !options.enabled) {
      return undefined;
    }
    let pointerId: number | null = null;
    let start = { x: 0, y: 0 };
    let timer: ReturnType<typeof setTimeout> | null = null;
    let tracker: SlideSeekTracker | null = null;
    let target: number | null = null;
    let menuTimer: ReturnType<typeof setTimeout> | null = null;

    const reset = () => {
      if (timer !== null) {
        clearTimeout(timer);
      }
      timer = null;
      pointerId = null;
      tracker = null;
      target = null;
      setScrub(null);
    };
    const onPointerDown = (event: PointerEvent) => {
      if (event.pointerType !== 'touch' || !event.isPrimary || !onVideoSurface(event.target)) {
        return;
      }
      reset();
      pointerId = event.pointerId;
      start = { x: event.clientX, y: event.clientY };
      timer = setTimeout(() => {
        timer = null;
        const range = latest.current.range();
        if (range === null) {
          pointerId = null;

          return;
        }
        tracker = new SlideSeekTracker(
          start.x,
          range.position,
          range.duration,
          element.getBoundingClientRect().width,
        );
      }, slideSeekHoldMs);
    };
    const onPointerMove = (event: PointerEvent) => {
      if (event.pointerId !== pointerId) {
        return;
      }
      // 長押しになる前に動かしたら、スワイプやスクロールに残す。
      if (tracker === null) {
        if (Math.hypot(event.clientX - start.x, event.clientY - start.y) >= slideSeekMoveLimit) {
          reset();
        }

        return;
      }
      const wasSliding = tracker.sliding;
      const seconds = tracker.move(event.clientX);
      if (seconds === null) {
        return;
      }
      if (!wasSliding) {
        latest.current.onSlideStart();
      }
      target = seconds;
      setScrub(seconds);
      latest.current.onScrub(seconds);
    };
    const onPointerUp = (event: PointerEvent) => {
      if (event.pointerId !== pointerId) {
        return;
      }
      if (target !== null) {
        latest.current.onCommit(target);
      } else if (tracker !== null) {
        // 指を離した後に、ブラウザがマウスの互換のイベント (mousedown、click) を送ることがある。開いたメニューが、
        // それをメニューの外のクリックとみなして閉じたり、指の下のメニューの項目を押したりしないよう、届き終えてから開く。
        const { x, y } = start;
        menuTimer = setTimeout(() => {
          menuTimer = null;
          latest.current.onLongPress(x, y);
        }, longPressMenuDelayMs);
      }
      reset();
    };
    const onPointerCancel = (event: PointerEvent) => {
      if (event.pointerId !== pointerId) {
        return;
      }
      if (target !== null) {
        latest.current.onCancel();
      }
      reset();
    };
    // 長押しで出るブラウザのメニュー (Android の contextmenu) は止め、離したときに開く。React の聞き手に届く前に止める。
    const onContextMenu = (event: MouseEvent) => {
      if (pointerId !== null) {
        event.preventDefault();
        event.stopPropagation();
      }
    };

    element.addEventListener('pointerdown', onPointerDown);
    element.addEventListener('pointermove', onPointerMove);
    element.addEventListener('pointerup', onPointerUp);
    element.addEventListener('pointercancel', onPointerCancel);
    element.addEventListener('contextmenu', onContextMenu, true);

    return () => {
      // 滑らせている間に使えなくなったら (ライブのバッファが使えなくなったなど)、元に戻す。
      if (target !== null) {
        latest.current.onCancel();
      }
      reset();
      if (menuTimer !== null) {
        clearTimeout(menuTimer);
      }
      element.removeEventListener('pointerdown', onPointerDown);
      element.removeEventListener('pointermove', onPointerMove);
      element.removeEventListener('pointerup', onPointerUp);
      element.removeEventListener('pointercancel', onPointerCancel);
      element.removeEventListener('contextmenu', onContextMenu, true);
    };
  }, [box, options.enabled]);

  return scrub;
}
