// ライブ視聴でタイムシフトのバッファを見ていた位置を、サービスごとに端末に覚える (#1600、設計書 5.7)。
// 同じサービスのライブ視聴に戻ったとき、その位置から続けて見られるようにする。
// 位置は、バッファの始まりが動いても指す場面が変わらないよう、放送時刻 (Unix ミリ秒) で持つ。
// バッファは家族で共有するが、どこまで見たかは端末を使う人のものなので、サーバーではなく端末に持つ。

// maxLiveResumes は、覚えておくサービスの数の上限。超えたら、最後に覚えたのが古いものから忘れる。
export const maxLiveResumes = 20;

const storageKey = 'kotatsu.player.liveResume';

// Entry は、サービス service で、放送時刻 at を見ていたことを、savedAt (Unix ミリ秒) に覚えたもの。
type Entry = { service: string; at: number; savedAt: number };

function isEntry(value: unknown): value is Entry {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const { service, at, savedAt } = value as Record<string, unknown>;

  return typeof service === 'string' && typeof at === 'number' && typeof savedAt === 'number';
}

// readEntries は、覚えているものを読む。読めないか、形が違えば、何も覚えていないものとして扱う。
function readEntries(storage: Pick<Storage, 'getItem'> | null): Entry[] {
  try {
    const raw = storage?.getItem(storageKey);
    if (raw == null) {
      return [];
    }
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed) || !parsed.every(isEntry)) {
      return [];
    }

    return parsed;
  } catch {
    return [];
  }
}

function writeEntries(storage: Pick<Storage, 'setItem'> | null, entries: Entry[]): void {
  try {
    storage?.setItem(storageKey, JSON.stringify(entries));
  } catch {
    // 書けなければ、戻ったときにライブの端から始めるだけ。
  }
}

// loadLiveResume は、サービス service で見ていた放送時刻を返す。覚えていなければ null を返す。
export function loadLiveResume(
  storage: Pick<Storage, 'getItem'> | null,
  service: string,
): number | null {
  return readEntries(storage).find((entry) => entry.service === service)?.at ?? null;
}

// saveLiveResume は、サービス service で、放送時刻 at を見ていたことを覚える。now は、覚えた時刻 (Unix ミリ秒)。
export function saveLiveResume(
  storage: Pick<Storage, 'getItem' | 'setItem'> | null,
  service: string,
  at: number,
  now: number,
): void {
  const entries = readEntries(storage).filter((entry) => entry.service !== service);
  entries.push({ service, at, savedAt: now });
  entries.sort((a, b) => a.savedAt - b.savedAt);
  writeEntries(storage, entries.slice(-maxLiveResumes));
}

// clearLiveResume は、サービス service で見ていた位置を忘れる。
export function clearLiveResume(
  storage: Pick<Storage, 'getItem' | 'setItem'> | null,
  service: string,
): void {
  const entries = readEntries(storage);
  const rest = entries.filter((entry) => entry.service !== service);
  if (rest.length !== entries.length) {
    writeEntries(storage, rest);
  }
}
