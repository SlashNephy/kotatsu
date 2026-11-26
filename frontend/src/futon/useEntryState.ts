import { useLocation, useRouter, type RouterHistory } from '@tanstack/react-router';
import { useEffect, useState, type SetStateAction } from 'react';

// 履歴の項目ごとに残す、画面の状態 (#2146)。一覧の「さらに読み込む」で読んだ分のように、URL には載せないが、
// ほかの画面から戻ったときに同じ状態で出したいものを持つ。ブラウザで戻ると、戻った項目の状態から始める。
// ページを読み込み直すと消える。
//
// 項目は、履歴の中の位置 (__TSR_index) で見分ける。URL を置き換えても位置は変わらないので、同じ項目のまま残る。
// 新しく移ると、その位置から先の項目は辿れなくなるので、残した状態を消す。

// maxEntries は、状態を残す項目の数の上限。古いものから消す。
const maxEntries = 100;

// entries は、履歴の中の位置ごとの、状態の名前と最後の状態。Map は足した順に並ぶので、先頭が最も古い。
const entries = new Map<number, Map<string, unknown>>();

function remember(index: number, name: string, value: unknown) {
  const states = entries.get(index) ?? new Map<string, unknown>();
  states.set(name, value);
  entries.delete(index);
  entries.set(index, states);
  for (const oldest of entries.keys()) {
    if (entries.size <= maxEntries) {
      break;
    }
    entries.delete(oldest);
  }
}

// tracked は、新しく移ったかを見ている履歴。
const tracked = new WeakSet<RouterHistory>();

// track は、新しく移ったときに、移った位置から先の項目の状態を消す。
function track(history: RouterHistory) {
  if (tracked.has(history)) {
    return;
  }
  tracked.add(history);
  history.subscribe(({ location, action }) => {
    if (action.type !== 'PUSH') {
      return;
    }
    const index = location.state.__TSR_index;
    for (const key of entries.keys()) {
      if (key >= index) {
        entries.delete(key);
      }
    }
  });
}

// useEntryState は、useState と同じく状態を返す。name は、同じ画面の中で状態を分ける名前。
// 状態は、今の履歴の項目に残す。開いたときは、その項目に残した状態から始め、なければ初めの値から始める。
// 同じ画面のままブラウザで戻ったり進んだりしたときは、その項目に残した状態に戻す。同じ画面のまま新しく移ったとき
// (番組の詳細を開くなど) は、今の状態を引き継ぐ。initial は、useState と同じく、値か、値を作る関数。
export function useEntryState<T>(
  name: string,
  initial: T | (() => T),
): [T, (action: SetStateAction<T>) => void] {
  const router = useRouter();
  const index = useLocation({ select: (location) => location.state.__TSR_index });
  const saved = (): { value: T } | null => {
    const states = entries.get(index);

    return states?.has(name) === true ? { value: states.get(name) as T } : null;
  };
  const [value, setValue] = useState<T>(
    () => saved()?.value ?? (typeof initial === 'function' ? (initial as () => T)() : initial),
  );
  const [shownIndex, setShownIndex] = useState(index);
  if (index !== shownIndex) {
    setShownIndex(index);
    const restored = saved();
    if (restored !== null) {
      setValue(restored.value);
    }
  }
  useEffect(() => {
    track(router.history);
  }, [router]);
  useEffect(() => {
    remember(index, name, value);
  }, [index, name, value]);

  return [value, setValue];
}
