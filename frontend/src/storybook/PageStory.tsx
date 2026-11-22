// Storybook で、アプリの画面 (SPA のパス) を、デモの応答で描く (#2056)。
// アプリと同じルート (routeTree) を、メモリの履歴で url から描く。応答は src/demo のデモのデータで、Story ごとに差し替えられる。
// ローカルの開発環境では起こしにくい条件 (読み込み中、失敗、空の一覧など) を、画面ごとに並べて見るのに使う。
import { Notifications } from '@mantine/notifications';
import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router';
import { useState } from 'react';
import { RelayEnvironmentProvider } from 'react-relay';

import { createDemoEnvironment, type DemoEnvironmentOptions } from '../demo';
import { routeTree } from '../routeTree.gen';

// PageStoryProps は、描く画面と、デモの応答の差し替え。
export type PageStoryProps = {
  // url は、描く画面のパス (/guide、/recordings?state=FAILED など)。
  url: string;
} & Pick<DemoEnvironmentOptions, 'overrides' | 'hold'>;

// PageStory は、url の画面を、デモの応答で描く。差し替えた応答は、Story を開き直すまで変えない。
export function PageStory({ url, overrides, hold }: PageStoryProps) {
  const [{ environment, router }] = useState(() => ({
    environment: createDemoEnvironment({ overrides, hold }).environment,
    router: createRouter({ routeTree, history: createMemoryHistory({ initialEntries: [url] }) }),
  }));

  return (
    <>
      <Notifications position="bottom-center" />
      <RelayEnvironmentProvider environment={environment}>
        <RouterProvider router={router} />
      </RelayEnvironmentProvider>
    </>
  );
}

// holdAll は、すべての操作を待たせる。読み込み中の画面を見るのに使う。
export const holdAll = () => true;

// failing は、呼ばれると投げる応答を返す。GraphQL のエラーとして画面に届く。
export function failing(message = 'the demo server failed') {
  return () => {
    throw new Error(message);
  };
}
