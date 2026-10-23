import { useEffect, useRef } from 'react';

import { keyGroup, RemoteKey, type KeyGroup } from './useDataBroadcast';

type KeyLike = Pick<
  KeyboardEvent,
  'key' | 'code' | 'altKey' | 'ctrlKey' | 'metaKey' | 'repeat' | 'defaultPrevented'
> & {
  target: EventTarget | null;
};

// altArrows は、⌥ (Alt) と一緒に押したときに、データ放送の矢印にする矢印のキー。
// 修飾キーのない矢印は、プレイヤーの音量とシークに残す (設計書 7.5)。
const altArrows: ReadonlyMap<string, RemoteKey> = new Map([
  ['ArrowUp', RemoteKey.up],
  ['ArrowDown', RemoteKey.down],
  ['ArrowLeft', RemoteKey.left],
  ['ArrowRight', RemoteKey.right],
]);

// altColors は、⌥ (Alt) と一緒に押したときに、データ放送の色のボタンにするキー (KeyboardEvent.code)。
// macOS では ⌥ と押すと key が別の文字になるので、code で見る。r はプレイヤーの読み直しに残す。
const altColors: ReadonlyMap<string, RemoteKey> = new Map([
  ['KeyB', RemoteKey.blue],
  ['KeyR', RemoteKey.red],
  ['KeyG', RemoteKey.green],
  ['KeyY', RemoteKey.yellow],
]);

// plainKeys は、修飾キーなしで押したときに、データ放送のボタンにするキー。プレイヤーのショートカットには使っていない。
const plainKeys: ReadonlyMap<string, RemoteKey> = new Map([
  ['Enter', RemoteKey.enter],
  ['Backspace', RemoteKey.back],
  ['0', RemoteKey.digit0],
  ['1', RemoteKey.digit1],
  ['2', RemoteKey.digit2],
  ['3', RemoteKey.digit3],
  ['4', RemoteKey.digit4],
  ['5', RemoteKey.digit5],
  ['6', RemoteKey.digit6],
  ['7', RemoteKey.digit7],
  ['8', RemoteKey.digit8],
  ['9', RemoteKey.digit9],
]);

// dataBroadcastKeyHelp は、ショートカットの一覧に出す、データ放送のキーの割り当て (#441)。並びは、リモコンのボタンと同じにする。
export const dataBroadcastKeyHelp = [
  { label: 'arrows', keys: ['Alt', '↑↓←→'] },
  { label: 'enter', keys: ['Enter'] },
  { label: 'back', keys: ['Backspace'] },
  { label: 'colors', keys: ['Alt', 'B', 'R', 'G', 'Y'] },
  { label: 'digits', keys: [{ from: '0', to: '9' }] },
] as const;

// dataBroadcastKeyFor は、データ放送を出している間に押したキーを、リモコンのボタンにする (#441)。割り当てがなければ null。
// 文字を入力している欄と、データ放送の文書がすでに受け取ったキー (defaultPrevented) は横取りしない。
// 押し続けたときに繰り返すのは、矢印だけにする。
export function dataBroadcastKeyFor(event: KeyLike): RemoteKey | null {
  if (event.defaultPrevented || event.ctrlKey || event.metaKey) {
    return null;
  }
  const target = event.target;
  if (
    target instanceof HTMLElement &&
    (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName))
  ) {
    return null;
  }
  if (event.altKey) {
    const arrow = altArrows.get(event.key);
    if (arrow !== undefined) {
      return arrow;
    }
    const color = altColors.get(event.code);

    return color !== undefined && !event.repeat ? color : null;
  }
  const key = plainKeys.get(event.key);

  return key !== undefined && !event.repeat ? key : null;
}

// useDataBroadcastKeys は、データ放送を操作できる間 (enabled)、キーボードでリモコンのボタンを押せるようにする。
// 文書が使わないキーのグループ (keyGroups にないもの) は押さない。
export function useDataBroadcastKeys(
  enabled: boolean,
  keyGroups: ReadonlySet<KeyGroup>,
  press: (key: RemoteKey) => void,
): void {
  const latest = useRef({ keyGroups, press });
  latest.current = { keyGroups, press };

  useEffect(() => {
    if (!enabled) {
      return;
    }
    const onKeyDown = (event: KeyboardEvent) => {
      const key = dataBroadcastKeyFor(event);
      if (key === null || !latest.current.keyGroups.has(keyGroup(key))) {
        return;
      }
      event.preventDefault();
      latest.current.press(key);
    };
    window.addEventListener('keydown', onKeyDown);

    return () => window.removeEventListener('keydown', onKeyDown);
  }, [enabled]);
}
