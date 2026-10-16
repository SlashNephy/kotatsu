// 選んだ画質を、端末ごとに覚えておく (#497)。回線や画面の大きさで、ちょうどよい画質が端末ごとに違うので、アカウントには保存しない。
// ライブと録画で同じものを使う。覚えた画質が、その画面で選べなければ、既定の経路に戻す (chooseRoute)。

const storageKey = 'kotatsu.player.quality';

// loadQuality は、覚えた画質の名前を読む。覚えていないか、読めなければ undefined を返す。
export function loadQuality(storage: Pick<Storage, 'getItem'> | null): string | undefined {
  try {
    const raw = storage?.getItem(storageKey);
    if (raw == null || raw === '') {
      return undefined;
    }

    return raw;
  } catch {
    // プライベートブラウジングなどで読めなくても、既定の経路で再生する。
    return undefined;
  }
}

// saveQuality は、選んだ画質の名前を覚える。書けなくても、再生は続ける。
export function saveQuality(storage: Pick<Storage, 'setItem'> | null, quality: string): void {
  try {
    storage?.setItem(storageKey, quality);
  } catch {
    // 書けなければ、次に開いたときは既定の経路になるだけ。
  }
}
