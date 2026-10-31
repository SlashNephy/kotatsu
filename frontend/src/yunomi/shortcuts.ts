import { useSyncExternalStore } from 'react';

import {
  playerCommands,
  simpleCommandIds,
  type PlayerCommandId,
  type SimpleCommandId,
} from './commands';
import { browserStorage } from './volume';

// プレイヤーのキーボードショートカット (設計書 7.5)。操作そのものは commands.ts の登録簿にあり、ここはキーの割り当てだけを持つ。
// 割り当ては、設定のページで端末ごとに変えられる (#810)。

// Keymap は、操作ごとに割り当てたキー (KeyboardEvent.key)。割り当てのない操作は、キーでは呼べない。
// 文字でないキーを Shift と一緒に押すときは、`Shift+` を前に付けて書く。文字のキーは、Shift で変わった文字 (? など) で書く。
export type Keymap = Readonly<Partial<Record<SimpleCommandId, readonly string[]>>>;

// defaultKeymap は、既定の割り当て。ショートカットの一覧も、割り当てから作る。
export const defaultKeymap: Keymap = {
  togglePlay: [' ', 'k'],
  seekBackward: ['ArrowLeft', 'j'],
  seekForward: ['ArrowRight', 'l'],
  // Alt+← (戻る) や Ctrl+← (単語の移動) は、ブラウザや OS とぶつかるので、Shift で大きく動かす (#498)。
  seekBackwardLong: ['Shift+ArrowLeft'],
  seekForwardLong: ['Shift+ArrowRight'],
  toggleCaptions: ['c'],
  toggleMute: ['m'],
  toggleFullscreen: ['f'],
  volumeUp: ['ArrowUp'],
  volumeDown: ['ArrowDown'],
  toggleDataBroadcast: ['d'],
  // b (back) で、直前に見ていた局へ戻る。もう一度押すと、元の局へ戻る (#816)。
  lastChannel: ['b'],
  capture: ['s'],
  restart: ['r'],
  toggleStats: ['i'],
  // ] で音を遅らせ、[ で戻す (#818)。
  audioDelayIncrease: [']'],
  audioDelayDecrease: ['['],
  showShortcuts: ['?'],
};

// keysFor は、操作に割り当てたキーを返す。割り当てがなければ空。
export function keysFor(id: PlayerCommandId, keymap: Keymap = defaultKeymap): readonly string[] {
  return (keymap as Partial<Record<PlayerCommandId, readonly string[]>>)[id] ?? [];
}

// actionsByKey は、キーから操作を引く表を作る。
function actionsByKey(keymap: Keymap): ReadonlyMap<string, SimpleCommandId> {
  return new Map(
    (Object.entries(keymap) as [SimpleCommandId, readonly string[]][]).flatMap(([action, keys]) =>
      keys.map((key) => [key, action] as const),
    ),
  );
}

type KeyLike = Pick<
  KeyboardEvent,
  'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey' | 'repeat'
> & {
  target: EventTarget | null;
};

// shortcutFor は、押したキーに割り当てた操作を返す。文字を入力している欄では、ショートカットにしない。
export function shortcutFor(
  event: KeyLike,
  keymap: Keymap = defaultKeymap,
): SimpleCommandId | null {
  const action = actionFor(event, actionsByKey(keymap));

  return action !== null && event.repeat && !playerCommands[action].repeatable ? null : action;
}

function actionFor(
  event: KeyLike,
  actions: ReadonlyMap<string, SimpleCommandId>,
): SimpleCommandId | null {
  if (event.ctrlKey || event.metaKey || event.altKey) {
    return null;
  }
  const target = event.target;
  if (
    target instanceof HTMLElement &&
    (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName))
  ) {
    return null;
  }
  // 文字でないキーは、Shift の割り当てがあればそれを、なければ Shift のないキーの割り当てを使う。
  if (event.shiftKey && event.key.length > 1) {
    const action = actions.get(`Shift+${event.key}`);
    if (action !== undefined) {
      return action;
    }
  }

  return actions.get(event.key) ?? null;
}

// modifierKeys は、それだけでは割り当てられない、修飾キーの名前。
const modifierKeys = new Set(['Shift', 'Control', 'Alt', 'Meta', 'AltGraph', 'CapsLock', 'Fn']);

// eventKey は、押したキーを、割り当てに書く形にする。修飾キーだけ、Ctrl、Alt、Meta と一緒のとき、
// 割り当てをやめるのに使う Escape は、null を返す。
export function eventKey(
  event: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>,
): string | null {
  if (
    event.ctrlKey ||
    event.metaKey ||
    event.altKey ||
    modifierKeys.has(event.key) ||
    event.key === 'Escape'
  ) {
    return null;
  }

  return event.shiftKey && event.key.length > 1 ? `Shift+${event.key}` : event.key;
}

// assignKey は、操作 id に key を足した割り当てを返す。ほかの操作に割り当てていたら、そちらから外す。
export function assignKey(keymap: Keymap, id: SimpleCommandId, key: string): Keymap {
  const next: Partial<Record<SimpleCommandId, readonly string[]>> = {};
  for (const [action, keys] of Object.entries(keymap) as [SimpleCommandId, readonly string[]][]) {
    next[action] = action === id ? keys : keys.filter((k) => k !== key);
  }
  const current = next[id] ?? [];
  next[id] = current.includes(key) ? current : [...current, key];

  return next;
}

// removeKey は、操作 id から key を外した割り当てを返す。
export function removeKey(keymap: Keymap, id: SimpleCommandId, key: string): Keymap {
  return { ...keymap, [id]: keysFor(id, keymap).filter((k) => k !== key) };
}

const storageKey = 'kotatsu.player.keymap';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:keymap';

// parseKeymap は、覚えた割り当てを読む。覚えた操作は、その割り当てで置き換える。読めなければ既定を返す。
export function parseKeymap(raw: string | null): Keymap {
  if (raw === null) {
    return defaultKeymap;
  }
  try {
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== 'object' || parsed === null) {
      return defaultKeymap;
    }
    const next: Partial<Record<SimpleCommandId, readonly string[]>> = { ...defaultKeymap };
    for (const id of simpleCommandIds) {
      const keys = (parsed as Record<string, unknown>)[id];
      if (Array.isArray(keys)) {
        next[id] = keys.filter((key): key is string => typeof key === 'string');
      }
    }

    return next;
  } catch {
    return defaultKeymap;
  }
}

function loadKeymapRaw(): string | null {
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

// useKeymap は、この端末の割り当てと、それを変える関数を返す。null を渡すと、既定に戻す。ほかのタブで変えても追う。
export function useKeymap(): [Keymap, (keymap: Keymap | null) => void] {
  // 文字列で購読し、描くたびに別の割り当てを作らないようにする。
  const raw = useSyncExternalStore(subscribe, loadKeymapRaw);

  return [
    parseKeymap(raw),
    (keymap) => {
      try {
        if (keymap === null) {
          browserStorage()?.removeItem(storageKey);
        } else {
          browserStorage()?.setItem(storageKey, JSON.stringify(keymap));
        }
      } catch {
        // 書けなければ、次に開いたときは既定に戻るだけ。
      }
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}
