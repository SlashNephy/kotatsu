/// <reference lib="webworker" />
import { cleanupOutdatedCaches, matchPrecache, precacheAndRoute } from 'workbox-precaching';
import { NavigationRoute, registerRoute } from 'workbox-routing';
import { NetworkOnly } from 'workbox-strategies';

import { notificationPath } from './notifications/link';

declare const self: ServiceWorkerGlobalScope;

// Service Worker は、アプリの外枠 (HTML、JS、CSS、アイコン) だけをキャッシュする (設計書 10.1)。
// 番組データと映像はキャッシュしない。API は Service Worker を通さずに、そのままサーバーに届く。

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
    { denylist: [/^\/graphql/, /^\/auth\//] },
  ),
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
  event.waitUntil(
    (async () => {
      const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
      const opened = windows[0];
      if (opened !== undefined) {
        await opened.focus();
        await opened.navigate(target);

        return;
      }
      await self.clients.openWindow(target);
    })(),
  );
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
