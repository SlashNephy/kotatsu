import { useEffect, useState, type FocusEvent, type PointerEvent } from 'react';

import { controlsHideDelayMs, shouldForceControlsVisible } from './controlsVisibility';

// controlsAttribute は、操作バーの要素に付ける印。タップが操作バーの中かどうかを見分けるのに使う。
export const controlsAttribute = 'data-player-controls';

export type ControlsVisibility = {
  visible: boolean;
  // containerProps は、プレイヤーを包む要素に渡す。
  containerProps: {
    onPointerMove: (event: PointerEvent<HTMLElement>) => void;
    onPointerDown: (event: PointerEvent<HTMLElement>) => void;
    onPointerLeave: (event: PointerEvent<HTMLElement>) => void;
    onFocusCapture: () => void;
    onBlurCapture: (event: FocusEvent<HTMLElement>) => void;
  };
};

// useControlsVisibility は、操作バーを出すかどうかを決める。マウスが動くと出し、操作がなくなって controlsHideDelayMs 経つと隠す。
// タッチでは pointermove が届かないので、映像をタップするたびに、出すと隠すを切り替える。
export function useControlsVisibility({ playing }: { playing: boolean }): ControlsVisibility {
  // 最後に動いた時刻ではなく、世代の番号を持つ。時刻だと、同じミリ秒の中の動きで数え直されない。
  const [generation, setGeneration] = useState(0);
  const [focusWithin, setFocusWithin] = useState(false);
  const [pointerLeft, setPointerLeft] = useState(false);
  const [timedOut, setTimedOut] = useState(false);
  const forceVisible = shouldForceControlsVisible({ playing, focusWithin });

  // 動いたときと、出し続ける条件が変わったときは、隠したことを取り消す。effect の中で毎回 setState すると
  // 描画が連鎖するので、描画の中で前の値と比べて合わせる (React の推奨する、状態の調整)。
  const [resetKey, setResetKey] = useState({ generation, forceVisible });
  if (resetKey.generation !== generation || resetKey.forceVisible !== forceVisible) {
    setResetKey({ generation, forceVisible });
    setTimedOut(false);
  }

  useEffect(() => {
    if (forceVisible) {
      return;
    }
    const timer = setTimeout(() => setTimedOut(true), controlsHideDelayMs);

    return () => clearTimeout(timer);
  }, [generation, forceVisible]);

  const visible = forceVisible || (!timedOut && !pointerLeft);
  const activate = () => {
    setPointerLeft(false);
    setGeneration((n) => n + 1);
  };

  return {
    visible,
    containerProps: {
      onPointerMove: (event) => {
        if (event.pointerType === 'mouse') {
          activate();
        }
      },
      onPointerDown: (event) => {
        if (event.pointerType === 'mouse') {
          activate();

          return;
        }
        // 操作バーの中のタップは、ボタンの操作なので、出したままにする。
        if (
          event.target instanceof Element &&
          event.target.closest(`[${controlsAttribute}]`) !== null
        ) {
          activate();

          return;
        }
        if (visible) {
          setTimedOut(true);
        } else {
          activate();
        }
      },
      // プレイヤーの外へ出たら、待たずに隠す。タッチでは、指を離しても pointerleave が届くので、隠さない。
      onPointerLeave: (event) => {
        if (event.pointerType === 'mouse') {
          setPointerLeft(true);
        }
      },
      onFocusCapture: () => setFocusWithin(true),
      onBlurCapture: (event) => {
        // フォーカスがプレイヤーの中で移っただけなら、出したままにする。
        const next = event.relatedTarget;
        if (next instanceof Node && event.currentTarget.contains(next)) {
          return;
        }
        setFocusWithin(false);
      },
    },
  };
}
