import { playerCommands, type PlayerCommandId, type SimpleCommandId } from './commands';

// プレイヤーのキーボードショートカット (設計書 7.5)。操作そのものは commands.ts の登録簿にあり、ここはキーの割り当てだけを持つ。

// shortcutKeys は、操作ごとに割り当てたキー (KeyboardEvent.key)。割り当てのない操作は、キーでは呼べない。
// ショートカットの一覧も、これから作る。
// 文字でないキーを Shift と一緒に押すときは、`Shift+` を前に付けて書く。文字のキーは、Shift で変わった文字 (? など) で書く。
export const shortcutKeys: Readonly<Partial<Record<SimpleCommandId, readonly string[]>>> = {
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
  showShortcuts: ['?'],
};

// keysFor は、操作に割り当てたキーを返す。割り当てがなければ空。
export function keysFor(id: PlayerCommandId): readonly string[] {
  return (shortcutKeys as Partial<Record<PlayerCommandId, readonly string[]>>)[id] ?? [];
}

const actionsByKey: ReadonlyMap<string, SimpleCommandId> = new Map(
  (Object.entries(shortcutKeys) as [SimpleCommandId, readonly string[]][]).flatMap(
    ([action, keys]) => keys.map((key) => [key, action] as const),
  ),
);

type KeyLike = Pick<
  KeyboardEvent,
  'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey' | 'repeat'
> & {
  target: EventTarget | null;
};

// shortcutFor は、押したキーに割り当てた操作を返す。文字を入力している欄では、ショートカットにしない。
export function shortcutFor(event: KeyLike): SimpleCommandId | null {
  const action = actionFor(event);

  return action !== null && event.repeat && !playerCommands[action].repeatable ? null : action;
}

function actionFor(event: KeyLike): SimpleCommandId | null {
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
