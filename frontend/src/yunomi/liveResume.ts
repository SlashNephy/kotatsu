// ライブ視聴でタイムシフトのバッファを見ていた位置を、サービスごとに端末に保存する (#1600、設計書 5.7)。
// 同じサービスのライブ視聴に戻ったとき、その位置から続けて見られるようにする。
// 位置は、バッファの始まりが動いても指す場面が変わらないよう、放送時刻 (Unix ミリ秒) で持つ。
// バッファは家族で共有するが、どこまで見たかは端末を使う人のものなので、サーバーではなく端末に持つ。

// maxLiveResumes は、保存しておくサービスの数の上限。超えたら、最後に保存したのが古いものから忘れる。
export const maxLiveResumes = 20;

const storageKey = 'kotatsu.player.liveResume';

// Entry は、サービス service で、放送時刻 at を見ていたことを、savedAt (Unix ミリ秒) に保存したもの。
type Entry = { service: string; at: number; savedAt: number };

function isEntry(value: unknown): value is Entry {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const { service, at, savedAt } = value as Record<string, unknown>;

  return typeof service === 'string' && typeof at === 'number' && typeof savedAt === 'number';
}

// readEntries は、保存しているものを読む。読めないか、形が違えば、何も保存していないものとして扱う。
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

// loadLiveResume は、サービス service で見ていた放送時刻を返す。保存していなければ null を返す。
export function loadLiveResume(
  storage: Pick<Storage, 'getItem'> | null,
  service: string,
): number | null {
  return readEntries(storage).find((entry) => entry.service === service)?.at ?? null;
}

// saveLiveResume は、サービス service で、放送時刻 at を見ていたことを保存する。now は、保存した時刻 (Unix ミリ秒)。
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
