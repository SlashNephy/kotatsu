import { useEffect, useSyncExternalStore } from 'react';

import { browserStorage } from '../yunomi/volume';

// アプリのアイコン (ブラウザのタブの favicon とヘッダーのロゴ) を、端末ごとに選ぶ (#1665)。
// インストールしたアイコン (マニフェストの icons と apple-touch-icon) は変えない。
// 絵は docs/icons の原本を frontend/public/icons に写したもので、同じオリジンの SVG として読む (CSP の img-src 'self')。

// appIconIds は、選べるアイコンの組。default は採用版 (main-v6.svg と favicon-mikan.svg)。
export const appIconIds = [
  'default',
  'mike-night',
  'mike-sunset',
  'chatora-night',
  'chatora-sunset',
] as const;

export type AppIconId = (typeof appIconIds)[number];

export const defaultAppIcon: AppIconId = 'default';

// appIconFiles は、組ごとの、ヘッダーのロゴ (app) と favicon の、public/icons の中のファイル名。
const appIconFiles: Record<AppIconId, { app: string; favicon: string }> = {
  default: { app: 'icon.svg', favicon: 'favicon.svg' },
  'mike-night': { app: 'p1-mike-night-app.svg', favicon: 'p1-mike-night-favicon.svg' },
  'mike-sunset': { app: 'p2-mike-sunset-app.svg', favicon: 'p2-mike-sunset-favicon.svg' },
  'chatora-night': { app: 'p3-chatora-night-app.svg', favicon: 'p3-chatora-night-favicon.svg' },
  'chatora-sunset': {
    app: 'p4-chatora-sunset-app.svg',
    favicon: 'p4-chatora-sunset-favicon.svg',
  },
};

// appIconUrls は、組 id の、ヘッダーのロゴと favicon の URL を返す。
export function appIconUrls(id: AppIconId): { app: string; favicon: string } {
  const files = appIconFiles[id];
  const base = `${import.meta.env.BASE_URL}icons/`;

  return { app: base + files.app, favicon: base + files.favicon };
}

// storageKey は、この端末で選んだアイコンを覚えるキー。ほかの端末とは同期しない。
const storageKey = 'kotatsu.appIcon';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:app-icon';

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
  try {
    storage?.setItem(storageKey, id);
  } catch {
    // 覚えられなければ、次に開いたときは既定に戻るだけ。
  }
  window.dispatchEvent(new Event(changeEvent));
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

function getSnapshot(): AppIconId {
  return loadAppIcon(browserStorage());
}

// useAppIcon は、この端末で選んだアイコンと、それを変える関数を返す。ほかのタブで変えても追う。
export function useAppIcon(): [AppIconId, (next: AppIconId) => void] {
  const id = useSyncExternalStore(subscribe, getSnapshot);

  return [id, (next) => saveAppIcon(browserStorage(), next)];
}

// applyFavicon は、ページの favicon (<link rel="icon">) を href に向ける。なければ足す。
export function applyFavicon(href: string): void {
  let link = document.head.querySelector<HTMLLinkElement>('link[rel~="icon"]');
  if (link === null) {
    link = document.createElement('link');
    link.rel = 'icon';
    link.type = 'image/svg+xml';
    document.head.append(link);
  }
  if (link.getAttribute('href') !== href) {
    link.href = href;
  }
}

// useAppFavicon は、選んだアイコンの favicon を、ページに当て続ける。すべての画面の枠 (RootLayout) で呼ぶ。
export function useAppFavicon(): void {
  const [id] = useAppIcon();
  const href = appIconUrls(id).favicon;
  useEffect(() => {
    applyFavicon(href);
  }, [href]);
}
