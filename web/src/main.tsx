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

import { cspNonce, cssVariablesResolver, theme } from './futon';
import { loadTimeZone } from './futon/timeZone';
import { createEnvironment } from './relay/environment';
import { routeTree } from './routeTree.gen';

// Mantine の Modal と Drawer は、背景のスクロールを止める <style> を react-remove-scroll で差し込む。
// react-remove-scroll は get-nonce から nonce を引くので、CSP の nonce を渡しておく (設計書 10.4)。
setNonce(cspNonce());

// basepath は、Vite の base に合わせる。tenban は / に置き、静的なデモはサブパスに置く (#832)。
const router = createRouter({ routeTree, basepath: import.meta.env.BASE_URL });
const environment = createEnvironment();

// Service Worker を登録する。新しい版があれば、次に開いたときに切り替わる。
void registerSW({ immediate: true });

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
        <RouterProvider router={router} />
      </RelayEnvironmentProvider>
    </MantineProvider>
  </StrictMode>,
);
