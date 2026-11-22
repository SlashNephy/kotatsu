import { useSyncExternalStore } from 'react';

import { preferenceChanged } from '../settings/syncedPreferences';

// ルールの一覧で折りたたんだグループの見出し (#1493) を、端末に覚える。ルールを開いてから戻っても、折りたたんだままにする。

const storageKey = 'kotatsu.rules.collapsedGroups';

// loadCollapsedGroups は、折りたたんだ見出しの鍵 (listFilter の sectionKey) を、端末から読む。読めなければ、すべて開く。
export function loadCollapsedGroups(): ReadonlySet<string> {
  return parseCollapsedGroups(rawSnapshot());
}

// parseCollapsedGroups は、端末に覚えた文字列 raw を、折りたたんだ見出しの鍵にする。読めなければ、すべて開く。
function parseCollapsedGroups(raw: string | null): ReadonlySet<string> {
  try {
    const parsed: unknown = JSON.parse(raw ?? '[]');

    return new Set(
      Array.isArray(parsed) ? parsed.filter((key): key is string => typeof key === 'string') : [],
    );
  } catch {
    return new Set();
  }
}

// saveCollapsedGroups は、折りたたんだ見出しの鍵を、端末に覚える。覚えられなくても、今の画面には効かせる。
export function saveCollapsedGroups(keys: ReadonlySet<string>): void {
  try {
    localStorage.setItem(storageKey, JSON.stringify([...keys]));
    preferenceChanged(storageKey);
  } catch {
    // 端末に覚えられないだけで、表示は変える。
  }
  window.dispatchEvent(new Event(changeEvent));
}

// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブと、ほかの端末から同期した値 (#1862) にしか届かない。
const changeEvent = 'kotatsu:collapsed-groups';

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

// useCollapsedGroups は、折りたたんだ見出しの鍵と、それを変える関数を返す。ほかのタブや、ほかの端末で変えても追う (#1862)。
// 読んだ文字列を手がかりにするので、値が変わらなければ、描き直さない。
export function useCollapsedGroups(): [ReadonlySet<string>, (next: ReadonlySet<string>) => void] {
  // 読んだ文字列から解く。引数なしで読み直すと、React Compiler が結果を覚えて使い回すからである。
  return [parseCollapsedGroups(useSyncExternalStore(subscribe, rawSnapshot)), saveCollapsedGroups];
}

function rawSnapshot(): string | null {
  try {
    return localStorage.getItem(storageKey);
  } catch {
    return null;
  }
}
