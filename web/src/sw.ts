/// <reference lib="webworker" />
import { clientsClaim } from 'workbox-core';
import { cleanupOutdatedCaches, matchPrecache, precacheAndRoute } from 'workbox-precaching';
import { NavigationRoute, registerRoute } from 'workbox-routing';
import { NetworkOnly } from 'workbox-strategies';

declare const self: ServiceWorkerGlobalScope;

// Service Worker は、アプリの外枠 (HTML、JS、CSS、フォント、アイコン) だけをキャッシュする (設計書 10.1)。
// 番組データと映像はキャッシュしない。API は Service Worker を通さずに、そのままサーバーに届く。
// プリキャッシュの一覧は、vite-plugin-pwa が self.__WB_MANIFEST に注入する。
precacheAndRoute(self.__WB_MANIFEST);
cleanupOutdatedCaches();

// 画面の移動は、まずサーバーに取りに行く。index.html には、リクエストごとの CSP の nonce が入るからである。
// オフラインのときだけ、プリキャッシュした外枠を返す。
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

void self.skipWaiting();
clientsClaim();
