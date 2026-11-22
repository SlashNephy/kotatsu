// Storybook で、アプリの画面 (SPA のパス) を、デモの応答で描く (#2056)。
// アプリと同じルート (routeTree) を、メモリの履歴で url から描く。応答は src/demo のデモのデータで、Story ごとに差し替えられる。
// ローカルの開発環境では起こしにくい条件 (読み込み中、失敗、空の一覧など) を、画面ごとに並べて見るのに使う。
import { Notifications } from '@mantine/notifications';
import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router';
import { useEffect, useState } from 'react';
import { RelayEnvironmentProvider } from 'react-relay';

import { createDemoEnvironment, type DemoEnvironmentOptions } from '../demo';
import type { Executor } from '../demo/executor';
import { routeTree } from '../routeTree.gen';

// PageStoryProps は、描く画面と、デモの応答の差し替え。
export type PageStoryProps = {
  // url は、描く画面のパス (/guide、/recordings?state=FAILED など)。デモのデータの ID (録画の ID など) は時刻から作るので、
  // パスに要るときは、デモの応答から読んでパスを返す関数を渡す。
  url: string | ((executor: Executor) => Promise<string>);
} & Pick<DemoEnvironmentOptions, 'overrides' | 'hold'>;

// PageStory は、url の画面を、デモの応答で描く。差し替えた応答は、Story を開き直すまで変えない。
export function PageStory({ url, overrides, hold }: PageStoryProps) {
  // デモが応えない画面 (管理の画面など) も枠を描けるよう、デモのデータにないフィールドは型の既定の値で埋める。
  const [{ environment, executor }] = useState(() =>
    createDemoEnvironment({ overrides, hold, fillMissing: true }),
  );
  const [router, setRouter] = useState<ReturnType<typeof createRouter> | null>(null);
  // failure は、パスを返す関数が失敗したときの理由。Story を空のまま止めず、理由を出す。
  const [failure, setFailure] = useState<string | null>(null);
  useEffect(() => {
    let cancelled = false;
    void (typeof url === 'string' ? Promise.resolve(url) : url(executor)).then(
      (path) => {
        if (!cancelled) {
          setRouter(
            createRouter({ routeTree, history: createMemoryHistory({ initialEntries: [path] }) }),
          );
        }
      },
      (error: unknown) => {
        if (!cancelled) {
          setFailure(error instanceof Error ? error.message : String(error));
        }
      },
    );

    return () => {
      cancelled = true;
    };
  }, [url, executor]);

  return (
    <>
      <Notifications position="bottom-center" />
      <RelayEnvironmentProvider environment={environment}>
        {failure !== null && <p role="alert">failed to open the story: {failure}</p>}
        {router !== null && <RouterProvider router={router} />}
      </RelayEnvironmentProvider>
    </>
  );
}

// firstRecordingPath は、デモの録画のうち、いちばん新しい録画の ID を、path の :id に入れたパスを返す。
export function firstRecordingPath(path: string) {
  return async (executor: Executor) => {
    const result = (await executor.execute(
      '{ recordings(first: 1) { edges { node { id } } } }',
      {},
    )) as { data?: { recordings?: { edges?: { node?: { id?: string } }[] } } };
    const id = result.data?.recordings?.edges?.[0]?.node?.id;
    if (id === undefined) {
      throw new Error('the demo has no recordings');
    }

    return path.replace(':id', id);
  };
}

// holdAll は、すべての操作を待たせる。読み込み中の画面を見るのに使う。
export const holdAll = () => true;

// failing は、呼ばれると投げる応答を返す。GraphQL のエラーとして画面に届く。
export function failing(message = 'the demo server failed') {
  return () => {
    throw new Error(message);
  };
}
