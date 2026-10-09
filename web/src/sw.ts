/// <reference lib="webworker" />
import { cleanupOutdatedCaches, matchPrecache, precacheAndRoute } from 'workbox-precaching';
import { NavigationRoute, registerRoute } from 'workbox-routing';
import { NetworkOnly } from 'workbox-strategies';

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
