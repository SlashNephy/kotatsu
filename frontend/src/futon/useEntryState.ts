import { useLocation } from '@tanstack/react-router';
import { useEffect, useState, type SetStateAction } from 'react';

// 履歴の項目ごとに残す、画面の状態 (#2146)。一覧の「さらに読み込む」で読んだ分のように、URL には載せないが、
// ほかの画面から戻ったときに同じ状態で出したいものを持つ。ブラウザで戻ると、戻った項目の状態から始める。
// ページを読み込み直すと消える。

// maxEntries は、残す状態の数の上限。古いものから消す。
const maxEntries = 50;

// entries は、履歴の項目の key と状態の名前ごとの、最後の状態。Map は足した順に並ぶので、先頭が最も古い。
const entries = new Map<string, unknown>();

function remember(key: string, value: unknown) {
  entries.delete(key);
  entries.set(key, value);
  for (const oldest of entries.keys()) {
    if (entries.size <= maxEntries) {
      break;
    }
    entries.delete(oldest);
  }
}

// useEntryState は、useState と同じく状態を返す。name は、同じ画面の中で状態を分ける名前。
// 状態は、今の履歴の項目に残す。画面を開いたときは、その項目に残した状態があれば、それから始める。
// URL を置き換えて履歴の項目が変わったときも、今の状態を新しい項目に残す。
// initial は、useState と同じく、値か、値を作る関数。
export function useEntryState<T>(
  name: string,
  initial: T | (() => T),
): [T, (action: SetStateAction<T>) => void] {
  const entry = useLocation({ select: (location) => location.state.__TSR_key });
  const key = `${entry ?? ''}:${name}`;
  const [value, setValue] = useState<T>(() =>
    entries.has(key)
      ? (entries.get(key) as T)
      : typeof initial === 'function'
        ? (initial as () => T)()
        : initial,
  );
  useEffect(() => {
    remember(key, value);
  }, [key, value]);

  return [value, setValue];
}
