import { preferenceChanged } from '../settings/syncedPreferences';

// ルールの一覧で折りたたんだグループの見出し (#1493) を、端末に覚える。ルールを開いてから戻っても、折りたたんだままにする。

const storageKey = 'kotatsu.rules.collapsedGroups';

// loadCollapsedGroups は、折りたたんだ見出しの鍵 (listFilter の sectionKey) を、端末から読む。読めなければ、すべて開く。
export function loadCollapsedGroups(): ReadonlySet<string> {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(storageKey) ?? '[]');

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
}
