import { useEffect, useRef, useState, type FocusEvent, type PointerEvent } from 'react';

import {
  controlsHideDelayMs,
  shouldForceControlsVisible,
  shouldHoldControls,
} from './controlsVisibility';

// controlsAttribute は、操作バーの要素に付ける印。タップが操作バーの中かどうかを見分けるのに使う。
export const controlsAttribute = 'data-player-controls';

const inControls = (target: EventTarget | null) =>
  target instanceof Element && target.closest(`[${controlsAttribute}]`) !== null;

export type ControlsVisibility = {
  visible: boolean;
  // containerProps は、プレイヤーを包む要素に渡す。
  containerProps: {
    onPointerMove: (event: PointerEvent<HTMLElement>) => void;
    onPointerDown: (event: PointerEvent<HTMLElement>) => void;
    onPointerLeave: (event: PointerEvent<HTMLElement>) => void;
    onFocusCapture: (event: FocusEvent<HTMLElement>) => void;
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
  // container は、プレイヤーを包む要素。開いている選択肢を探すのに使う。
  // pointerOverControls と pressingControls は、マウスが操作バーの上にあるか、操作バーの中を押したままか。
  const container = useRef<HTMLElement | null>(null);
  const pointerOverControls = useRef(false);
  const pressingControls = useRef(false);
  // pointerLeftRef は、pointerLeft を、隠す時間の判定から読むための写し。
  const pointerLeftRef = useRef(false);
  pointerLeftRef.current = pointerLeft;
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
    const timer = setTimeout(() => {
      // 操作バーのメニューと選択肢は、操作バーの中に描く (withinPortal={false})。開いている間だけ、その要素がある。
      const menuOpened =
        container.current?.querySelector(
          `[${controlsAttribute}] :is([aria-expanded="true"], [role="menu"], [role="listbox"])`,
        ) != null;
      const hold = shouldHoldControls({
        pointerOverControls: pointerOverControls.current,
        pressingControls: pressingControls.current,
        menuOpened,
      });
      // 使っている最中なら、隠さずに数え直す。プレイヤーの外へ出て隠しているなら、数え直さない。
      if (hold && !pointerLeftRef.current) {
        setGeneration((n) => n + 1);
      } else {
        setTimedOut(true);
      }
    }, controlsHideDelayMs);

    return () => clearTimeout(timer);
  }, [generation, forceVisible]);

  // 押したまま操作バーの外で離しても、押していないことにする。
  useEffect(() => {
    const onPointerUp = () => {
      pressingControls.current = false;
    };
    window.addEventListener('pointerup', onPointerUp);
    window.addEventListener('pointercancel', onPointerUp);

    return () => {
      window.removeEventListener('pointerup', onPointerUp);
      window.removeEventListener('pointercancel', onPointerUp);
    };
  }, []);

  const visible = forceVisible || (!timedOut && !pointerLeft);
  const activate = () => {
    setPointerLeft(false);
    setGeneration((n) => n + 1);
  };

  return {
    visible,
    containerProps: {
      onPointerMove: (event) => {
        container.current = event.currentTarget;
        if (event.pointerType === 'mouse') {
          pointerOverControls.current = inControls(event.target);
          activate();
        }
      },
      onPointerDown: (event) => {
        container.current = event.currentTarget;
        // タッチでシークバーを動かしている間も、隠さない。
        pressingControls.current = inControls(event.target);
        if (event.pointerType === 'mouse') {
          activate();

          return;
        }
        // 操作バーの中のタップは、ボタンの操作なので、出したままにする。
        if (inControls(event.target)) {
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
        // シークバーを掴んだまま外へ出ても、離すまでは隠さない。
        if (event.pointerType === 'mouse' && !pressingControls.current) {
          pointerOverControls.current = false;
          setPointerLeft(true);
        }
      },
      // キーボードで操作しているときだけ、出し続ける。マウスでボタンを押したときもフォーカスは残るが、
      // そのままだと、プレイヤーの外へマウスを出しても隠れなくなる。
      onFocusCapture: (event) => {
        // キーボードだけで操作したときも、開いているメニューを探せるようにする。
        container.current = event.currentTarget;
        setFocusWithin(event.target instanceof Element && event.target.matches(':focus-visible'));
      },
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
