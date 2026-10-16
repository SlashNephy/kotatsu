// プレイヤーのキーボードショートカット (設計書 7.5)。

export type ShortcutAction =
  | 'togglePlay'
  | 'seekBackward'
  | 'seekForward'
  | 'toggleCaptions'
  | 'toggleMute'
  | 'toggleFullscreen'
  | 'showShortcuts';

// shortcutKeys は、操作ごとに割り当てたキー (KeyboardEvent.key)。ショートカットの一覧も、これから作る。
export const shortcutKeys: Readonly<Record<ShortcutAction, readonly string[]>> = {
  togglePlay: [' ', 'k'],
  seekBackward: ['ArrowLeft', 'j'],
  seekForward: ['ArrowRight', 'l'],
  toggleCaptions: ['c'],
  toggleMute: ['m'],
  toggleFullscreen: ['f'],
  showShortcuts: ['?'],
};

const actionsByKey: ReadonlyMap<string, ShortcutAction> = new Map(
  (Object.entries(shortcutKeys) as [ShortcutAction, readonly string[]][]).flatMap(
    ([action, keys]) => keys.map((key) => [key, action] as const),
  ),
);

type KeyLike = Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'repeat'> & {
  target: EventTarget | null;
};

// repeatable は、キーを押し続けたときに、繰り返してよい操作。切り替える操作は、押すたびに 1 回だけにする。
const repeatable: ReadonlySet<ShortcutAction> = new Set(['seekBackward', 'seekForward']);

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
  return actionsByKey.get(event.key) ?? null;
}
