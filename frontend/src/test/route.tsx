import { MantineProvider } from '@mantine/core';
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  Outlet,
  RouterProvider,
  type AnyRoute,
} from '@tanstack/react-router';
import { act, render } from '@testing-library/react';
import { RelayEnvironmentProvider } from 'react-relay';
import {
  Environment,
  Network,
  Observable,
  RecordSource,
  Store,
  type GraphQLResponse,
  type RequestParameters,
  type Variables,
} from 'relay-runtime';

import '../i18n';
import { createRootValue } from '../demo/data';
import { createEventHub } from '../demo/events';
import { createExecutor } from '../demo/executor';

// Respond は、画面が送った GraphQL の操作に応える。undefined を返すと、応答を返さず待たせる。
export type Respond = (
  request: RequestParameters,
  variables: Variables,
) => GraphQLResponse | Error | undefined;

// createTestEnvironment は、respond で応える Relay の環境を作る。
export function createTestEnvironment(respond: Respond): Environment {
  return new Environment({
    network: Network.create((request, variables) =>
      Observable.create((sink) => {
        const result = respond(request, variables);
        if (result === undefined) {
          return;
        }
        if (result instanceof Error) {
          sink.error(result);

          return;
        }
        sink.next(result);
        sink.complete();
      }),
    ),
    store: new Store(new RecordSource()),
  });
}

// createDemoEnvironment は、デモ (src/demo) のデータを返す Relay の環境を作る。overrides のフィールドは、デモの値より優先する。
// スキーマは tenban と同じ schema.graphql なので、画面が送る query をそのまま検証できる。
export function createDemoEnvironment(overrides: object = {}): Environment {
  const executor = createExecutor({ ...createRootValue(createEventHub()), ...overrides });

  return new Environment({
    network: Network.create(
      async (request, variables) =>
        (await executor.execute(request.text ?? '', variables)) as GraphQLResponse,
    ),
    store: new Store(new RecordSource()),
  });
}

// renderRoute は、ファイルのルート route を、アプリの枠 (__root) を除いて、本物のルーターで url に描く。ルートを読み終えるまで待つ。
// id は createFileRoute に渡したパス。routeTree.gen.ts と同じく、ルートの ID (Route.useParams などが引く) をそれにして、root の直下に付ける。
// ID の末尾の _ は、親のレイアウトに入れないための印なので、パスからは除く。
export async function renderRoute(
  route: AnyRoute,
  id: string,
  url: string,
  environment: Environment,
) {
  const root = createRootRoute({ component: Outlet });
  route.update({
    id,
    path: id.replace(/_(?=\/|$)/g, ''),
    getParentRoute: () => root,
  } as never);
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: [url] }),
  });
  // env="test" は、Mantine の動きの遷移とポータルを止め、開いたメニューをすぐに描く。
  const result = render(
    <MantineProvider env="test">
      <RelayEnvironmentProvider environment={environment}>
        <RouterProvider router={router} />
      </RelayEnvironmentProvider>
    </MantineProvider>,
  );
  await act(() => router.load());

  return { router, ...result };
}
