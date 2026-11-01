// ライブ視聴で見た局の履歴 (#816、設計書 7.5)。端末ごとに覚え、直前の局へ戻る操作と、選局の欄の「最近」に使う。
// 見る局は端末ごとに違うので、アカウントには保存しない。

// maxRecentChannels は、覚えておく局の数。
export const maxRecentChannels = 10;

const storageKey = 'kotatsu.live.recentChannels';

// loadRecentChannels は、見た局の ID を、新しい順に返す。読めないか、壊れていれば空にする。
export function loadRecentChannels(storage: Pick<Storage, 'getItem'> | null): string[] {
  try {
    const raw = storage?.getItem(storageKey);
    if (raw == null) {
      return [];
    }
    const parsed: unknown = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      return parsed
        .filter((id): id is string => typeof id === 'string')
        .slice(0, maxRecentChannels);
    }
  } catch {
    // プライベートブラウジングなどで読めなくても、履歴がないものとして続ける。
  }

  return [];
}

// recordChannel は、見た局を履歴の先頭に置いた、新しい履歴を返す。同じ局は 1 つにまとめる。
export function recordChannel(recent: readonly string[], id: string): string[] {
  return [id, ...recent.filter((other) => other !== id)].slice(0, maxRecentChannels);
}

// saveRecentChannels は、履歴を覚える。書けなくても、視聴は続ける。
export function saveRecentChannels(
  storage: Pick<Storage, 'setItem'> | null,
  recent: readonly string[],
): void {
  try {
    storage?.setItem(storageKey, JSON.stringify(recent));
  } catch {
    // 書けなければ、直前の局へ戻れないだけ。
  }
}

// otherRecentChannels は、履歴のうち、今見ている局と、選べない局 (番組表にない局など) を除いたものを、新しい順に返す。
// 先頭が、直前に見ていた局。
export function otherRecentChannels(
  recent: readonly string[],
  current: string,
  available: ReadonlySet<string>,
): string[] {
  return recent.filter((id) => id !== current && available.has(id));
}
