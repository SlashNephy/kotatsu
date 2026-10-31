import { useEffect, useRef, useSyncExternalStore } from 'react';

import {
  playerCommands,
  runCommand,
  simpleCommandIds,
  type PlayerCommands,
  type SimpleCommandId,
} from './commands';
import { browserStorage } from './volume';

// Gamepad API でのプレイヤーの操作 (#810、設計書 7.5)。テレビにつないだ PC で、ゲームパッドをリモコンの代わりに使う。
// ボタンの番号は、Gamepad API の標準の配置 (standard mapping) で数える。割り当ては、キーと同じく端末ごとに変えられる。

// GamepadMap は、操作ごとに割り当てたボタンの番号。
export type GamepadMap = Readonly<Partial<Record<SimpleCommandId, readonly number[]>>>;

// defaultGamepadMap は、既定の割り当て。十字キーで移動と音量、A で再生と一時停止、LB と RB で局を移る。
export const defaultGamepadMap: GamepadMap = {
  togglePlay: [0],
  toggleMute: [1],
  toggleCaptions: [2],
  toggleDataBroadcast: [3],
  previousChannel: [4],
  nextChannel: [5],
  seekBackwardLong: [6],
  seekForwardLong: [7],
  lastChannel: [8],
  toggleFullscreen: [9],
  volumeUp: [12],
  volumeDown: [13],
  seekBackward: [14],
  seekForward: [15],
};

// buttonLabels は、標準の配置のボタンの名前。Xbox のコントローラーの表記に合わせる。
const buttonLabels: readonly string[] = [
  'A',
  'B',
  'X',
  'Y',
  'LB',
  'RB',
  'LT',
  'RT',
  'Back',
  'Start',
  'L3',
  'R3',
  '↑',
  '↓',
  '←',
  '→',
  'Home',
];

// buttonLabel は、ボタンの番号を、画面に出す名前にする。標準の配置にないボタンは、番号のまま出す。
export function buttonLabel(button: number): string {
  return buttonLabels[button] ?? String(button);
}

// newlyPressed は、前の状態 previous から、今 current までに押されたボタンの番号を返す。
export function newlyPressed(previous: readonly boolean[], current: readonly boolean[]): number[] {
  return current.flatMap((pressed, i) => (pressed && previous[i] !== true ? [i] : []));
}

// commandForButton は、ボタンに割り当てた操作を返す。割り当てがなければ null。
export function commandForButton(map: GamepadMap, button: number): SimpleCommandId | null {
  for (const [id, buttons] of Object.entries(map) as [SimpleCommandId, readonly number[]][]) {
    if (buttons.includes(button)) {
      return id;
    }
  }

  return null;
}

// repeatDelayMs は、押し続けてから繰り返し始めるまで、repeatIntervalMs は、繰り返す間隔。キーボードの繰り返しに近づける。
const repeatDelayMs = 400;
const repeatIntervalMs = 150;

// repeatDue は、heldSince から押し続けているボタンを、lastFired の後に、now でもう一度呼ぶかどうか。
export function repeatDue(heldSince: number, lastFired: number, now: number): boolean {
  return now - heldSince >= repeatDelayMs && now - lastFired >= repeatIntervalMs;
}

// assignButton は、操作 id にボタンを足した割り当てを返す。ほかの操作に割り当てていたら、そちらから外す。
export function assignButton(map: GamepadMap, id: SimpleCommandId, button: number): GamepadMap {
  const next: Partial<Record<SimpleCommandId, readonly number[]>> = {};
  for (const [action, buttons] of Object.entries(map) as [SimpleCommandId, readonly number[]][]) {
    next[action] = action === id ? buttons : buttons.filter((b) => b !== button);
  }
  const current = next[id] ?? [];
  next[id] = current.includes(button) ? current : [...current, button];

  return next;
}

// removeButton は、操作 id からボタンを外した割り当てを返す。
export function removeButton(map: GamepadMap, id: SimpleCommandId, button: number): GamepadMap {
  return { ...map, [id]: (map[id] ?? []).filter((b) => b !== button) };
}

const storageKey = 'kotatsu.player.gamepadMap';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:gamepad-map';

// parseGamepadMap は、覚えた割り当てを読む。覚えていない操作は既定のボタンにするが、覚えた操作が使っているボタンは外す。
// 読めなければ既定を返す。
export function parseGamepadMap(raw: string | null): GamepadMap {
  if (raw === null) {
    return defaultGamepadMap;
  }
  try {
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== 'object' || parsed === null) {
      return defaultGamepadMap;
    }
    const stored: Partial<Record<SimpleCommandId, readonly number[]>> = {};
    for (const id of simpleCommandIds) {
      const buttons = (parsed as Record<string, unknown>)[id];
      if (Array.isArray(buttons)) {
        stored[id] = buttons.filter((b): b is number => Number.isInteger(b));
      }
    }
    const used = new Set(Object.values(stored).flat());
    const next: Partial<Record<SimpleCommandId, readonly number[]>> = {};
    for (const id of simpleCommandIds) {
      const buttons = stored[id] ?? defaultGamepadMap[id]?.filter((b) => !used.has(b));
      if (buttons !== undefined) {
        next[id] = buttons;
      }
    }

    return next;
  } catch {
    return defaultGamepadMap;
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

// useGamepadMap は、この端末の割り当てと、それを変える関数を返す。null を渡すと、既定に戻す。
export function useGamepadMap(): [GamepadMap, (map: GamepadMap | null) => void] {
  // 文字列で購読し、描くたびに別の割り当てを作らないようにする。
  const raw = useSyncExternalStore(subscribe, loadRaw);

  return [
    parseGamepadMap(raw),
    (map) => {
      try {
        if (map === null) {
          browserStorage()?.removeItem(storageKey);
        } else {
          browserStorage()?.setItem(storageKey, JSON.stringify(map));
        }
      } catch {
        // 書けなければ、次に開いたときは既定に戻るだけ。
      }
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}

// connectedGamepads は、つないでいるゲームパッド。Gamepad API がなければ空。
export function connectedGamepads(): Gamepad[] {
  if (typeof navigator === 'undefined' || typeof navigator.getGamepads !== 'function') {
    return [];
  }

  return navigator.getGamepads().filter((pad): pad is Gamepad => pad !== null);
}

// useGamepadButtons は、ゲームパッドをつないでいる間、毎フレームボタンを読み、押したボタンと押し続けているボタンを渡す。
// ゲームパッドの操作は、ボタンを押してから届くので (ブラウザは、押すまでゲームパッドを見せない)、つないだ知らせを待って読み始める。
export function useGamepadButtons(
  onPress: (button: number) => void,
  onHold?: (button: number, heldSince: number, now: number) => void,
): void {
  const handlers = useRef({ onPress, onHold });
  handlers.current = { onPress, onHold };

  useEffect(() => {
    let frame: number | null = null;
    // previous は、ゲームパッドごとの、前のフレームのボタンの状態。heldSince は、押し始めた時刻。
    const previous = new Map<number, boolean[]>();
    const heldSince = new Map<string, number>();
    const poll = () => {
      frame = null;
      const pads = connectedGamepads();
      const now = performance.now();
      for (const pad of pads) {
        const current = pad.buttons.map((button) => button.pressed);
        for (const button of newlyPressed(previous.get(pad.index) ?? [], current)) {
          heldSince.set(`${pad.index}:${button}`, now);
          handlers.current.onPress(button);
        }
        current.forEach((pressed, button) => {
          const key = `${pad.index}:${button}`;
          const since = heldSince.get(key);
          if (!pressed) {
            heldSince.delete(key);
          } else if (since !== undefined) {
            handlers.current.onHold?.(button, since, now);
          }
        });
        previous.set(pad.index, current);
      }
      if (pads.length > 0) {
        frame = requestAnimationFrame(poll);
      }
    };
    const start = () => {
      frame ??= requestAnimationFrame(poll);
    };
    window.addEventListener('gamepadconnected', start);
    // 画面を開く前からつないでいて、ボタンを押してあれば、すぐに読み始める。
    if (connectedGamepads().length > 0) {
      start();
    }

    return () => {
      window.removeEventListener('gamepadconnected', start);
      if (frame !== null) {
        cancelAnimationFrame(frame);
      }
    };
  }, []);
}

// usePlayerGamepad は、ゲームパッドで、プレイヤーを操作できるようにする。今使えない操作のボタンは、何もしない。
// 押し続けたときは、繰り返してよい操作 (シーク、音量) だけを繰り返す。
export function usePlayerGamepad(commands: PlayerCommands): void {
  const commandsRef = useRef(commands);
  commandsRef.current = commands;
  const [map] = useGamepadMap();
  const mapRef = useRef(map);
  mapRef.current = map;
  // lastFired は、ボタンごとの、最後に操作を呼んだ時刻。
  const lastFired = useRef(new Map<number, number>());

  useGamepadButtons(
    (button) => {
      const id = commandForButton(mapRef.current, button);
      if (id !== null) {
        lastFired.current.set(button, performance.now());
        runCommand(commandsRef.current, id);
      }
    },
    (button, heldSince, now) => {
      const id = commandForButton(mapRef.current, button);
      if (id === null || !playerCommands[id].repeatable) {
        return;
      }
      if (repeatDue(heldSince, lastFired.current.get(button) ?? heldSince, now)) {
        lastFired.current.set(button, now);
        runCommand(commandsRef.current, id);
      }
    },
  );
}
