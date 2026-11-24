// 端末に保存する設定のうち、端末をまたいで同期するものを、サーバーと揃える (#1862、設計書 9.1)。
// 各設定は、今までどおり localStorage を読み書きし、ここが localStorage とサーバーの間をコピーする。
// - 画面を開いたら、サーバーの値を localStorage にコピーし、storage イベントで各設定に通知する。
// - 設定を変えたら (preferenceChanged)、少し待ってまとめてサーバーへ送る。
// - サーバーにまだないキーは、この端末で初めて揃えるときだけ、この端末の値を送って引き継ぐ。
// 機器や回線によって変わる設定 (音量、画質、拡大など) と、端末で見ていた位置は、端末ごとに保存するので入れない。

// syncedPreferenceKeys は、同期するキー。サーバーの backend/internal/web/graph/preferences.go と同じものにする。
export const syncedPreferenceKeys: ReadonlySet<string> = new Set([
  'kotatsu.appIcon',
  'kotatsu.anime.streamingServices',
  'kotatsu.capture.fileNameTemplate',
  'kotatsu.guide.appearance',
  'kotatsu.guide.extendedHours',
  'kotatsu.player.audioLanguage',
  'kotatsu.player.autoPictureInPicture',
  'kotatsu.player.cmAutoSkip',
  'kotatsu.player.commentOffsetMs',
  'kotatsu.player.gamepadMap',
  'kotatsu.player.keymap',
  'kotatsu.player.miniPlayer',
  'kotatsu.player.mouseActions',
  'kotatsu.player.resetAudioDelayOnChannelChange',
  'kotatsu.player.swipeControls',
  'kotatsu.rules.collapsedGroups',
]);

// PreferenceChange は、サーバーへ送る変更。value が null なら、サーバーの値を消す。
export type PreferenceChange = { key: string; value: string | null };

// Uploader は、変更をサーバーへ送り、終わったら (失敗しても) done を呼ぶ。
export type Uploader = (changes: PreferenceChange[], done: () => void) => void;

// uploadDelayMs は、変えてから送るまで待つ時間。続けて変えたものを 1 回で送る。
const uploadDelayMs = 300;

let uploader: Uploader | null = null;
// pending は、まだ送っていない変更のキー。inFlight は、送って返事を待っているキー。
// どちらのキーも、サーバーから読み直した値で上書きしない。古い値で、変えたばかりの値を戻さないためである。
const pending = new Set<string>();
const inFlight = new Set<string>();
let timer: ReturnType<typeof setTimeout> | null = null;
// sending は、送って返事を待っているかどうか。送るのは一度に 1 回だけにし、返事が来てから次を送る。
// 重ねて送ると、先の返事で inFlight のマークが早く外れたり、サーバーに着く順が入れ替わったりするからである。
let sending = false;

// takenOverKey は、この端末の設定をサーバーへ引き継いだことを保存するキー。引き継ぎは端末ごとに 1 回だけにする。
// 開くたびに引き継ぐと、ほかの端末で消した (既定に戻した) 設定を、この端末が開いたときに送り直してしまうからである。
const takenOverKey = 'kotatsu.preferences.takenOver';

function read(storage: Pick<Storage, 'getItem'> | null, key: string): string | null {
  try {
    return storage?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

function localStorageOrNull(): Storage | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

function schedule(): void {
  if (uploader === null || timer !== null || sending || pending.size === 0) {
    return;
  }
  timer = setTimeout(flush, uploadDelayMs);
}

function flush(): void {
  timer = null;
  const send = uploader;
  if (send === null || pending.size === 0) {
    return;
  }
  const storage = localStorageOrNull();
  const keys = [...pending];
  pending.clear();
  for (const key of keys) {
    inFlight.add(key);
  }
  sending = true;
  send(
    keys.map((key) => ({ key, value: read(storage, key) })),
    () => {
      for (const key of keys) {
        inFlight.delete(key);
      }
      sending = false;
      // 待っている間に変えた設定を送る。
      schedule();
    },
  );
}

// preferenceChanged は、設定 key を localStorage に書いた (消した) ことを通知する。同期するキーなら、サーバーへ送る。
// 送るのは、送るときの localStorage の値なので、続けて変えれば最後の値だけを送る。
export function preferenceChanged(key: string): void {
  if (!syncedPreferenceKeys.has(key)) {
    return;
  }
  pending.add(key);
  schedule();
}

// registerPreferenceUploader は、サーバーへ送る関数を置く。ログインしている間だけ置き、外すための関数を返す。
export function registerPreferenceUploader(next: Uploader): () => void {
  uploader = next;
  schedule();

  return () => {
    if (uploader === next) {
      uploader = null;
    }
  };
}

// applyServerPreferences は、サーバーの値 preferences を storage にコピーし、変えたキーを storage イベントで通知する。
// この端末で初めて揃えるときは、サーバーになく、この端末にある値を送って引き継ぐ。それ以外のときにサーバーにない値は、この端末でも消す。
// サーバーにある値は、初めて揃えるときもサーバーの値を使う。先に揃えた端末の値が、ほかの端末に写る。
export function applyServerPreferences(
  preferences: readonly { key: string; value: string }[],
  storage: Pick<Storage, 'getItem' | 'setItem' | 'removeItem'> | null,
): void {
  const server = new Map(preferences.map(({ key, value }) => [key, value]));
  const first = read(storage, takenOverKey) === null;
  try {
    storage?.setItem(takenOverKey, 'true');
  } catch {
    // 書けなければ、次に開いたときも引き継ぐ。サーバーにある値は上書きしないので、害は小さい。
  }
  for (const key of syncedPreferenceKeys) {
    if (pending.has(key) || inFlight.has(key)) {
      continue;
    }
    const local = read(storage, key);
    const remote = server.get(key) ?? null;
    if (local === remote) {
      continue;
    }
    if (remote === null && first) {
      preferenceChanged(key);
      continue;
    }
    try {
      if (remote === null) {
        storage?.removeItem(key);
      } else {
        storage?.setItem(key, remote);
      }
    } catch {
      // 書けなければ、この端末では前の値のまま使う。
      continue;
    }
    window.dispatchEvent(new StorageEvent('storage', { key, newValue: remote }));
  }
}

// resetSyncedPreferencesForTest は、テストのために、保持している状態を破棄する。
export function resetSyncedPreferencesForTest(): void {
  uploader = null;
  pending.clear();
  inFlight.clear();
  sending = false;
  if (timer !== null) {
    clearTimeout(timer);
    timer = null;
  }
}
