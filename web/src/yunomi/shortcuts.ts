// プレイヤーのキーボードショートカット (設計書 7.5)。

export type ShortcutAction =
  | 'togglePlay'
  | 'seekBackward'
  | 'seekForward'
  | 'seekBackwardLong'
  | 'seekForwardLong'
  | 'toggleCaptions'
  | 'toggleMute'
  | 'toggleFullscreen'
  | 'volumeUp'
  | 'volumeDown'
  | 'toggleDataBroadcast'
  | 'capture'
  | 'restart'
  | 'toggleStats'
  | 'showShortcuts';

// shortcutKeys は、操作ごとに割り当てたキー (KeyboardEvent.key)。ショートカットの一覧も、これから作る。
// 文字でないキーを Shift と一緒に押すときは、`Shift+` を前に付けて書く。文字のキーは、Shift で変わった文字 (? など) で書く。
export const shortcutKeys: Readonly<Record<ShortcutAction, readonly string[]>> = {
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
  capture: ['s'],
  restart: ['r'],
  toggleStats: ['i'],
  showShortcuts: ['?'],
};

const actionsByKey: ReadonlyMap<string, ShortcutAction> = new Map(
  (Object.entries(shortcutKeys) as [ShortcutAction, readonly string[]][]).flatMap(
    ([action, keys]) => keys.map((key) => [key, action] as const),
  ),
);

type KeyLike = Pick<
  KeyboardEvent,
  'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey' | 'repeat'
> & {
  target: EventTarget | null;
};

// repeatable は、キーを押し続けたときに、繰り返してよい操作。切り替える操作は、押すたびに 1 回だけにする。
const repeatable: ReadonlySet<ShortcutAction> = new Set([
  'seekBackward',
  'seekForward',
  'seekBackwardLong',
  'seekForwardLong',
  'volumeUp',
  'volumeDown',
]);

// shortcutFor は、押したキーに割り当てた操作を返す。文字を入力している欄では、ショートカットにしない。
export function shortcutFor(event: KeyLike): ShortcutAction | null {
  const action = actionFor(event);

  return action !== null && event.repeat && !repeatable.has(action) ? null : action;
}

function actionFor(event: KeyLike): ShortcutAction | null {
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
    const action = actionsByKey.get(`Shift+${event.key}`);
    if (action !== undefined) {
      return action;
    }
  }

  return actionsByKey.get(event.key) ?? null;
}
