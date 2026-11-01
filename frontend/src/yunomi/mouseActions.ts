import { useEffect, useRef, useState, useSyncExternalStore } from 'react';

import { runCommand, type PlayerCommands, type SimpleCommandId } from './commands';
import { onVideoSurface } from './useControlsVisibility';
import { browserStorage } from './volume';

// マウスのホイールと中クリックでの、プレイヤーの操作 (#810、設計書 7.5)。TVTest に合わせて、既定では、
// 映像の上のホイールで音量を変え、中クリックでミュートを切り替える。どちらも端末ごとに変えられる。

// WheelAction は、ホイールで何をするか。volume は音量、seek は 10 秒のシーク、none は何もしない (ページを流す)。
export type WheelAction = 'volume' | 'seek' | 'none';
// MiddleClickAction は、中クリックで何をするか。
export type MiddleClickAction = 'toggleMute' | 'toggleFullscreen' | 'togglePlay' | 'none';

export const wheelActions: readonly WheelAction[] = ['volume', 'seek', 'none'];
export const middleClickActions: readonly MiddleClickAction[] = [
  'toggleMute',
  'toggleFullscreen',
  'togglePlay',
  'none',
];

export type MouseActions = { wheel: WheelAction; middleClick: MiddleClickAction };

export const defaultMouseActions: MouseActions = { wheel: 'volume', middleClick: 'toggleMute' };

// wheelStep は、1 回の操作に当たるホイールの量 (deltaY)。マウスの 1 目盛りはおおむね 100 で、
// トラックパッドは細かく届くので、溜めてから 1 回にする。
const wheelStep = 100;

// wheelCommands は、溜めたホイールの量 accumulated から、呼ぶ操作と、残りの量を返す。
// 上へ回す (負) と音量を上げるか戻り、下へ回す (正) と音量を下げるか進む。
export function wheelCommands(
  action: WheelAction,
  accumulated: number,
): { command: SimpleCommandId | null; rest: number } {
  if (action === 'none') {
    return { command: null, rest: 0 };
  }
  if (Math.abs(accumulated) < wheelStep) {
    return { command: null, rest: accumulated };
  }
  const up = accumulated < 0;
  const command: SimpleCommandId =
    action === 'volume' ? (up ? 'volumeUp' : 'volumeDown') : up ? 'seekBackward' : 'seekForward';

  return { command, rest: accumulated - Math.sign(accumulated) * wheelStep };
}

const storageKey = 'kotatsu.player.mouseActions';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:mouse-actions';

// parseMouseActions は、覚えた設定を読む。読めない値は既定にする。
export function parseMouseActions(raw: string | null): MouseActions {
  if (raw === null) {
    return defaultMouseActions;
  }
  try {
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== 'object' || parsed === null) {
      return defaultMouseActions;
    }
    const { wheel, middleClick } = parsed as Record<string, unknown>;

    return {
      wheel: wheelActions.find((a) => a === wheel) ?? defaultMouseActions.wheel,
      middleClick:
        middleClickActions.find((a) => a === middleClick) ?? defaultMouseActions.middleClick,
    };
  } catch {
    return defaultMouseActions;
  }
}

function loadRaw(): string | null {
  try {
    return browserStorage()?.getItem(storageKey) ?? null;
  } catch {
    return null;
  }
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

// useMouseActions は、この端末の設定と、それを変える関数を返す。ほかのタブで変えても追う。
export function useMouseActions(): [MouseActions, (actions: MouseActions) => void] {
  const raw = useSyncExternalStore(subscribe, loadRaw);

  return [
    parseMouseActions(raw),
    (actions) => {
      try {
        browserStorage()?.setItem(storageKey, JSON.stringify(actions));
      } catch {
        // 書けなければ、次に開いたときは既定に戻るだけ。
      }
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}

// usePlayerMouse は、プレイヤーを包む要素で、映像の上のホイールと中クリックを、設定の操作にする。返す ref を、その要素に渡す。
// 要素は、別のウィンドウ (Document Picture-in-Picture) に移すと作り直されるので、ref で受けて、変わるたびに付け直す。
// 操作バーの上では、ブラウザの動きのままにする。呼ぶ操作がないときも、ページのスクロールは止めない。
export function usePlayerMouse(commands: PlayerCommands): (element: HTMLElement | null) => void {
  const [actions] = useMouseActions();
  const latest = useRef({ actions, commands });
  latest.current = { actions, commands };
  const [element, setElement] = useState<HTMLElement | null>(null);

  useEffect(() => {
    if (element === null) {
      return;
    }
    let accumulated = 0;
    const onWheel = (event: WheelEvent) => {
      const { actions, commands } = latest.current;
      if (
        actions.wheel === 'none' ||
        event.deltaY === 0 ||
        !onVideoSurface(event.target) ||
        event.ctrlKey
      ) {
        return;
      }
      // 呼ぶ操作がなければ (シークできないライブなど)、ページを流すのを止めない。
      const { command } = wheelCommands(actions.wheel, Math.sign(event.deltaY) * wheelStep);
      if (command === null || commands[command] === undefined) {
        return;
      }
      event.preventDefault();
      // 行やページの単位で届いたとき (Firefox) は、1 回の知らせを 1 目盛りとみなす。
      accumulated +=
        event.deltaMode === WheelEvent.DOM_DELTA_PIXEL
          ? event.deltaY
          : Math.sign(event.deltaY) * wheelStep;
      for (;;) {
        const next = wheelCommands(actions.wheel, accumulated);
        accumulated = next.rest;
        if (next.command === null) {
          break;
        }
        runCommand(commands, next.command);
      }
    };
    // 中ボタンを押すと出る、ブラウザの自動スクロールを止める。
    const onMouseDown = (event: MouseEvent) => {
      if (
        event.button === 1 &&
        latest.current.actions.middleClick !== 'none' &&
        onVideoSurface(event.target)
      ) {
        event.preventDefault();
      }
    };
    const onAuxClick = (event: MouseEvent) => {
      const { actions, commands } = latest.current;
      if (event.button !== 1 || actions.middleClick === 'none' || !onVideoSurface(event.target)) {
        return;
      }
      event.preventDefault();
      runCommand(commands, actions.middleClick);
    };
    element.addEventListener('wheel', onWheel, { passive: false });
    element.addEventListener('mousedown', onMouseDown);
    element.addEventListener('auxclick', onAuxClick);

    return () => {
      element.removeEventListener('wheel', onWheel);
      element.removeEventListener('mousedown', onMouseDown);
      element.removeEventListener('auxclick', onAuxClick);
    };
  }, [element]);

  return setElement;
}
