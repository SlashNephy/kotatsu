// 選んだ画質を、端末ごとに保存しておく (#497)。回線や画面の大きさで、ちょうどよい画質が端末ごとに違うので、アカウントには保存しない。
// ライブと録画で同じものを使う。保存した画質が、その画面で選べなければ、既定の経路に戻す (chooseRoute)。

const storageKey = 'kotatsu.player.quality';

// loadQuality は、保存した画質の名前を読む。保存していないか、読めなければ undefined を返す。
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

// saveQuality は、選んだ画質の名前を保存する。書けなくても、再生は続ける。
export function saveQuality(storage: Pick<Storage, 'setItem'> | null, quality: string): void {
  try {
    storage?.setItem(storageKey, quality);
  } catch {
    // 書けなければ、次に開いたときは既定の経路になるだけ。
  }
}
