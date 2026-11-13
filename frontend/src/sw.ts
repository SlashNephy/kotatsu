/// <reference lib="webworker" />
import { cleanupOutdatedCaches, matchPrecache, precacheAndRoute } from 'workbox-precaching';
import { NavigationRoute, registerRoute } from 'workbox-routing';
import { CacheFirst, NetworkOnly } from 'workbox-strategies';

import { notificationPath } from './notifications/link';
import { openNotificationTarget } from './notifications/openTarget';

declare const self: ServiceWorkerGlobalScope;

// Service Worker は、アプリの外枠 (HTML、JS、CSS、アイコン、ブラウザの中での変換の WebAssembly) をキャッシュする (設計書 10.1)。
// 番組データと映像はキャッシュしない。API は Service Worker を通さずに、そのままサーバーに届く。
// 端末に保存した録画 (#480) は、画面が OPFS に書いて読むので、Service Worker は関わらない。

// 画面の移動は、まずサーバーに取りに行く。index.html には、リクエストごとの CSP の nonce が入るからである。
// オフラインのときだけ、プリキャッシュした外枠を返す。
// Workbox は先に登録したルートを使うので、/ と /index.html をプリキャッシュから返さないよう、
// プリキャッシュのルートより先に登録する。
const appShell = 'index.html';
registerRoute(
  new NavigationRoute(
    async (options) => {
      try {
        return await new NetworkOnly().handle(options);
      } catch (error) {
        const cached = await matchPrecache(appShell);
        if (cached === undefined) {
          throw error;
        }

        return cached;
      }
    },
    // /internal の下 (OIDC と視聴記録のサービスから戻るリダイレクトなど) と /api の下 (署名付き URL のダウンロードなど) は、
    // 画面ではないので返さない (#1605)。
    { denylist: [/^\/internal\//, /^\/api\//] },
  ),
);

// 字幕を描くフォントは、基本の字 (1.6 MB ほど) と、JIS X 0213 で足された漢字 (1 MB ほど) の 2 つのファイルに分けてある (#1520)。
// 大きいのでプリキャッシュせず、一度読んだら残す。端末に保存した録画 (#480) をオフラインで再生するときも、字幕を描けるようにするためである。
// ファイル名は版ごとに変わるので、それぞれ最後に読んだ 1 つだけを残す。片方を読んだときに、もう片方は消さない。
const broadcastFontCache = 'kotatsu-broadcast-font';
const broadcastFontPattern = /\/assets\/rounded-mplus-1m-wadalab-mix-(base|jisx0213)-[^/]*\.woff2$/;
registerRoute(
  ({ url }) => url.origin === self.location.origin && broadcastFontPattern.test(url.pathname),
  new CacheFirst({
    cacheName: broadcastFontCache,
    plugins: [
      {
        cacheDidUpdate: async ({ cacheName, request }) => {
          const kind = broadcastFontPattern.exec(new URL(request.url).pathname)?.[1];
          const cache = await caches.open(cacheName);
          for (const key of await cache.keys()) {
            // 分ける前の 1 つの TTF も、同じキャッシュに残っていれば消す。
            const keyKind = broadcastFontPattern.exec(new URL(key.url).pathname)?.[1];
            if (key.url !== request.url && (keyKind === kind || keyKind === undefined)) {
              await cache.delete(key);
            }
          }
        },
      },
    ],
  }),
);

// プリキャッシュの一覧は、vite-plugin-pwa が self.__WB_MANIFEST に注入する。
// 新しい版の Service Worker は、開いているタブがすべて閉じてから有効になる。
// それまでは、開いているタブが読み込む古い版のファイルを、古いキャッシュから返し続ける。
precacheAndRoute(self.__WB_MANIFEST);
cleanupOutdatedCaches();

// Web Push の通知を出す (設計書 6.2)。本文は、tenban が組み立てて送る (webpush.Payload)。
// url は、通知を押したときに開く画面のパス (視聴予約の通知では、そのサービスのライブ視聴)。
type PushPayload = { title: string; body: string; eventId: string; kind: string; url?: string };

self.addEventListener('push', (event) => {
  const payload = parsePush(event.data);
  if (payload === null) {
    return;
  }
  // 同じ出来事の通知は、届き直しても 1 つにまとめる。
  event.waitUntil(
    self.registration.showNotification(payload.title, {
      body: payload.body,
      tag: payload.eventId,
      icon: '/icons/icon-192.png',
      data: { kind: payload.kind, url: notificationPath(payload.url) },
    }),
  );
});

// 通知を押したら、開いているタブがあればそこで、なければ新しいタブで、通知が示す画面を開く。
// 示す画面がなければ、通知の一覧を開く。
self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const data = event.notification.data as { url?: unknown } | null;
  const target =
    notificationPath(typeof data?.url === 'string' ? data.url : null) ?? '/notifications';
  event.waitUntil(openNotificationTarget(self.clients, target));
});

// parsePush は、Web Push の本文を読む。壊れた本文なら null を返す。
function parsePush(data: PushMessageData | null): PushPayload | null {
  try {
    const payload: unknown = data?.json();
    if (
      typeof payload !== 'object' ||
      payload === null ||
      typeof (payload as PushPayload).title !== 'string'
    ) {
      return null;
    }

    return payload as PushPayload;
  } catch {
    return null;
  }
}
