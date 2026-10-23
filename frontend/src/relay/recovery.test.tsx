import { act, cleanup, render, screen } from '@testing-library/react';
import { Suspense } from 'react';
import { RelayEnvironmentProvider } from 'react-relay';
import {
  Environment,
  Network,
  Observable,
  RecordSource,
  Store,
  type FetchFunction,
  type GraphQLResponse,
} from 'relay-runtime';
import { afterEach, describe, expect, test, vi } from 'vitest';

import UserMenuQueryNode, { type UserMenuQuery } from '../auth/__generated__/UserMenuQuery.graphql';
import { ErrorBoundary, ErrorRecoveryContext, type ErrorRecovery } from '../futon';

import { createConnectionTracker } from './connection';
import { GraphQLRequestError } from './requestError';
import { isTransientError } from './transient';
import { useLazyLoadQuery } from './useLazyLoadQuery';

const response: GraphQLResponse = {
  data: {
    viewer: { id: 'VXNlcjox', displayName: 'みかん', username: 'mikan', avatarUrl: null },
    version: { version: '2026.10.1', revision: null, modified: false, latestRelease: null },
  },
};

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function Viewer() {
  const data = useLazyLoadQuery<UserMenuQuery>(UserMenuQueryNode, {});

  return <p>{data.viewer.displayName}</p>;
}

// setup は、errors を順に返し、尽きたら応答を返すサーバーで、Viewer を ErrorBoundary の中に描く。
function setup(errors: Error[]) {
  vi.spyOn(console, 'error').mockImplementation(() => undefined);
  const tracker = createConnectionTracker();
  const recovery: ErrorRecovery = {
    recoveries: tracker.recoveries,
    subscribe: tracker.subscribe,
    isRecoverable: isTransientError,
  };
  const fetchFn = vi.fn<FetchFunction>(() =>
    Observable.create((sink) => {
      const error = errors.shift();
      if (error === undefined) {
        sink.next(response);
        sink.complete();
      } else {
        sink.error(error);
      }
    }),
  );
  const environment = new Environment({
    network: Network.create(fetchFn),
    store: new Store(new RecordSource()),
  });
  render(
    <RelayEnvironmentProvider environment={environment}>
      <ErrorRecoveryContext value={recovery}>
        <ErrorBoundary fallback={(error) => <p>failed: {error.message}</p>}>
          <Suspense fallback={<p>loading</p>}>
            <Viewer />
          </Suspense>
        </ErrorBoundary>
      </ErrorRecoveryContext>
    </RelayEnvironmentProvider>,
  );

  // recover は、つなぎ直していた接続がつながったことにして、サーバーが戻ったと知らせる。
  const recover = () => {
    const connection = tracker.track();
    connection.reconnecting();
    connection.connected();
  };

  return { fetchFn, recover };
}

describe('サーバーが戻ったときの描き直し', () => {
  test('一時的な障害で読み込めなかった画面は、サーバーが戻ったら、送り直して自動で描き直す', async () => {
    const { fetchFn, recover } = setup([
      new GraphQLRequestError('unexpected response (HTTP 503)', 503, undefined),
    ]);
    expect(await screen.findByText('failed: unexpected response (HTTP 503)')).toBeDefined();

    act(recover);

    expect(await screen.findByText('みかん')).toBeDefined();
    // Relay は失敗を覚えているので、読み込みのキーを変えなければ、送り直さない。
    expect(fetchFn).toHaveBeenCalledTimes(2);
  });

  test('送り直しても変わらない失敗は、サーバーが戻っても描き直さない', async () => {
    const { fetchFn, recover } = setup([
      new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED'),
    ]);
    expect(await screen.findByText('failed: unauthenticated')).toBeDefined();

    act(recover);

    expect(screen.getByText('failed: unauthenticated')).toBeDefined();
    expect(fetchFn).toHaveBeenCalledTimes(1);
  });

  test('失敗する前にサーバーが戻ったことは、描き直す合図にしない', async () => {
    const { recover } = setup([
      new GraphQLRequestError('unexpected response (HTTP 503)', 503, undefined),
      new GraphQLRequestError('unexpected response (HTTP 503)', 503, undefined),
    ]);
    expect(await screen.findByText('failed: unexpected response (HTTP 503)')).toBeDefined();

    // 1 回目の回復で描き直し、また失敗する。
    act(recover);
    expect(await screen.findByText('failed: unexpected response (HTTP 503)')).toBeDefined();

    // 2 回目の失敗の後の回復で、もう一度描き直して通る。
    act(recover);
    expect(await screen.findByText('みかん')).toBeDefined();
  });
});
