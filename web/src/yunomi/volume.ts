// プレイヤーの音量を、端末ごとに覚えておく。端末によって、ちょうどよい音量が違うので、アカウントには保存しない。

export type Volume = {
  // volume は、0 から 1 の音量。
  volume: number;
  muted: boolean;
};

// defaultVolume は、覚えた音量がないときの音量。
export const defaultVolume: Volume = { volume: 1, muted: false };

const storageKey = 'kotatsu.player.volume';

// loadVolume は、覚えた音量を読む。読めないか、壊れていれば、既定の音量を返す。
export function loadVolume(storage: Pick<Storage, 'getItem'> | null): Volume {
  try {
    const raw = storage?.getItem(storageKey);
    if (raw == null) {
      return defaultVolume;
    }
    const parsed: unknown = JSON.parse(raw);
    if (
      typeof parsed === 'object' &&
      parsed !== null &&
      'volume' in parsed &&
      'muted' in parsed &&
      typeof parsed.volume === 'number' &&
      Number.isFinite(parsed.volume) &&
      typeof parsed.muted === 'boolean'
    ) {
      return { volume: Math.min(Math.max(parsed.volume, 0), 1), muted: parsed.muted };
    }
  } catch {
    // プライベートブラウジングなどで読めなくても、既定の音量で再生する。
  }

  return defaultVolume;
}

// saveVolume は、音量を覚える。書けなくても、再生は続ける。
export function saveVolume(storage: Pick<Storage, 'setItem'> | null, volume: Volume): void {
  try {
    storage?.setItem(storageKey, JSON.stringify(volume));
  } catch {
    // 書けなければ、次に開いたときは既定の音量になるだけ。
  }
}

// browserStorage は、localStorage を返す。触れるだけで例外を投げるブラウザもあるので、そのときは null を返す。
export function browserStorage(): Storage | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}
