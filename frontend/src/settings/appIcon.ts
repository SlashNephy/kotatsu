import { useSyncExternalStore } from 'react';

import { browserStorage } from '../yunomi/volume';
import { preferenceChanged } from './syncedPreferences';

// ヘッダーのロゴに出すアイコンを選ぶ (#1665)。端末をまたいで同期する (#1862)。
// タブの favicon は、16px では柄の違いが見えないので、どの組でもみかんの favicon のままにする (#759)。
// インストールしたアイコン (マニフェストの icons と apple-touch-icon) も変えない。
// 絵は docs/icons の原本を frontend/public/icons に写したもので、同じオリジンの SVG として読む (CSP の img-src 'self')。

// appIconIds は、選べるアイコンの組。default は採用版 (main-v6.svg と favicon-mikan.svg) の三毛猫。
// ほかの組は、採用版の構図のまま、猫の柄だけを変えたもの (docs/icons の coat-*.svg)。
export const appIconIds = [
  'default',
  'chatora',
  'kijitora',
  'sabatora',
  'kuro',
  'shiro',
  'hachiware',
] as const;

export type AppIconId = (typeof appIconIds)[number];

export const defaultAppIcon: AppIconId = 'default';

// appIconFiles は、組ごとの、ヘッダーのロゴの、public/icons の中のファイル名。
const appIconFiles: Record<AppIconId, string> = {
  default: 'icon.svg',
  chatora: 'coat-chatora.svg',
  kijitora: 'coat-kijitora.svg',
  sabatora: 'coat-sabatora.svg',
  kuro: 'coat-kuro.svg',
  shiro: 'coat-shiro.svg',
  hachiware: 'coat-hachiware.svg',
};

// appIconUrl は、組 id の、ヘッダーのロゴの URL を返す。
export function appIconUrl(id: AppIconId): string {
  return `${import.meta.env.BASE_URL}icons/${appIconFiles[id]}`;
}

// storageKey は、選んだアイコンを覚えるキー。端末をまたいで同期する (#1862)。
const storageKey = 'kotatsu.appIcon';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:app-icon';

// unsaved は、端末に覚えられなかったときに、このタブの中だけで使う組。覚えられたら、端末の値に戻す。
let unsaved: AppIconId | null = null;

function isAppIconId(value: string): value is AppIconId {
  return (appIconIds as readonly string[]).includes(value);
}

// loadAppIcon は、この端末で選んだアイコンを読む。覚えていないか、知らない値か、読めなければ既定の組を返す。
export function loadAppIcon(storage: Pick<Storage, 'getItem'> | null): AppIconId {
  try {
    const stored = storage?.getItem(storageKey);
    if (stored != null && isAppIconId(stored)) {
      return stored;
    }
  } catch {
    // プライベートブラウジングなどで読めなくても、既定のアイコンを出す。
  }

  return defaultAppIcon;
}

// saveAppIcon は、選んだアイコンを覚え、同じタブの中の画面に知らせる。覚えられなくても、今の画面には効かせる。
export function saveAppIcon(storage: Pick<Storage, 'setItem'> | null, id: AppIconId): void {
  // 覚えられなければ、このタブの中だけで使う。次に開いたときは、端末に覚えた組 (なければ既定) に戻る。
  unsaved = id;
  try {
    if (storage !== null) {
      storage.setItem(storageKey, id);
      preferenceChanged(storageKey);
      unsaved = null;
    }
  } catch {
    // unsaved に残した組を使う。
  }
  window.dispatchEvent(new Event(changeEvent));
}

function subscribe(onChange: () => void): () => void {
  // ほかのタブで選び直したら、そちらに従う。
  const onStorage = () => {
    unsaved = null;
    onChange();
  };
  window.addEventListener('storage', onStorage);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onStorage);
    window.removeEventListener(changeEvent, onChange);
  };
}

// currentAppIcon は、このタブで使う組。覚えられなかった組があれば、それを使う。
export function currentAppIcon(): AppIconId {
  return unsaved ?? loadAppIcon(browserStorage());
}

// useAppIcon は、この端末で選んだアイコンと、それを変える関数を返す。ほかのタブで変えても追う。
export function useAppIcon(): [AppIconId, (next: AppIconId) => void] {
  const id = useSyncExternalStore(subscribe, currentAppIcon);

  return [id, (next) => saveAppIcon(browserStorage(), next)];
}
