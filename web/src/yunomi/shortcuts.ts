// プレイヤーのキーボードショートカット (設計書 7.5)。

export type ShortcutAction =
  | 'togglePlay'
  | 'seekBackward'
  | 'seekForward'
  | 'toggleCaptions'
  | 'toggleMute'
  | 'toggleFullscreen';

type KeyLike = Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'altKey'> & {
  target: EventTarget | null;
};

// shortcutFor は、押したキーに割り当てた操作を返す。文字を入力している欄では、ショートカットにしない。
export function shortcutFor(event: KeyLike): ShortcutAction | null {
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
  switch (event.key) {
    case ' ':
    case 'k':
      return 'togglePlay';
    case 'ArrowLeft':
    case 'j':
      return 'seekBackward';
    case 'ArrowRight':
    case 'l':
      return 'seekForward';
    case 'c':
      return 'toggleCaptions';
    case 'm':
      return 'toggleMute';
    case 'f':
      return 'toggleFullscreen';
    default:
      return null;
  }
}
