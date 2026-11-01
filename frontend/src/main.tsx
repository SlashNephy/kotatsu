// 本文と見出しの丸ゴシック体。アプリで使う太さ (400、500、700。600 は 700 で描く) だけを読む。
import '@fontsource/m-plus-rounded-1c/400.css';
import '@fontsource/m-plus-rounded-1c/500.css';
import '@fontsource/m-plus-rounded-1c/700.css';
import '@mantine/core/styles.css';
import './futon/global.css';
import './i18n';

import { MantineProvider } from '@mantine/core';
import { createRouter, RouterProvider } from '@tanstack/react-router';
import { setNonce } from 'get-nonce';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { RelayEnvironmentProvider } from 'react-relay';
import { registerSW } from 'virtual:pwa-register';

import { removeLegacyHistory } from './captures/upload';
import { cspNonce, cssVariablesResolver, ErrorRecoveryContext, theme } from './futon';
import { loadTimeZone } from './futon/timeZone';
import { createEnvironment } from './relay/environment';
import { errorRecovery } from './relay/recovery';
import { routeTree } from './routeTree.gen';

// Mantine の Modal と Drawer は、背景のスクロールを止める <style> を react-remove-scroll で差し込む。
// react-remove-scroll は get-nonce から nonce を引くので、CSP の nonce を渡しておく (設計書 10.4)。
setNonce(cspNonce());

// basepath は、Vite の base に合わせる。tenban は / に置き、静的なデモはサブパスに置く (#832)。
const router = createRouter({ routeTree, basepath: import.meta.env.BASE_URL });
// 静的なデモ (#832) では、tenban の代わりに、ブラウザの中で応答を返す。
// 条件は import.meta.env.MODE を直に書く。通常のビルドでは定数の比較として消え、デモのコードを含まない。
const environment =
  import.meta.env.MODE === 'demo' ? (await import('./demo')).installDemo() : createEnvironment();

// Service Worker を登録する。新しい版があれば、次に開いたときに切り替わる。
// デモは、ライブ視聴の映像を流す Service Worker を別に登録するので、登録しない。
if (import.meta.env.MODE !== 'demo') {
  void registerSW({ immediate: true });
}

// 端末に残していたキャプチャの履歴 (#457) は、サーバーの一覧に置き換えたので、移さずに消す (#908)。
removeLegacyHistory();

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}

const root = document.getElementById('root');
if (root === null) {
  throw new Error('the root element is missing');
}

// 日時をサーバーのタイムゾーンで表すため、読み込んでから画面を出す (#504)。
await loadTimeZone();

createRoot(root).render(
  <StrictMode>
    <MantineProvider
      theme={theme}
      cssVariablesResolver={cssVariablesResolver}
      defaultColorScheme="auto"
      getStyleNonce={cspNonce}
    >
      <RelayEnvironmentProvider environment={environment}>
        {/* サーバーの入れ替わりなどで読み込みに失敗した画面を、サーバーが戻ったら自動で描き直す。 */}
        <ErrorRecoveryContext value={errorRecovery}>
          <RouterProvider router={router} />
        </ErrorRecoveryContext>
      </RelayEnvironmentProvider>
    </MantineProvider>
  </StrictMode>,
);
