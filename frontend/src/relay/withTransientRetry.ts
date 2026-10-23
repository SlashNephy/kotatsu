import { Observable, type FetchFunction, type GraphQLResponse } from 'relay-runtime';

import { retryDelayMs, serverConnections, type ConnectionTracker } from './connection';
import { isTransientError } from './transient';

// maxRetries は、一時的な障害で送り直す回数の上限。retryDelayMs の間隔 (1 秒から倍々、30 秒まで) で、
// 合わせて 5 分ほど試す。サーバーの更新で Pod が入れ替わる間は、これで足りる。
// これを使い切っても通らなければ、本当の障害とみなして、エラーを画面に出す。
export const maxRetries = 12;

export type TransientRetryOptions = {
  tracker?: ConnectionTracker;
  random?: () => number;
};

// withTransientRetry は、一時的な障害 (isTransientError) で失敗した query を、間をおいて送り直す FetchFunction を返す。
// 送り直している間、Relay からは応答を待っているように見えるので、画面は読み込み中のままで、エラーを出さない。
// 送り直している間は、tracker に知らせ、ヘッダーに印を出す。送り直して通ったら、tracker がサーバーが戻ったことを知らせる。
// mutation は、同じ操作を 2 回行うおそれがあるので、送り直さない。
export function withTransientRetry(
  fetchFn: FetchFunction,
  options: TransientRetryOptions = {},
): FetchFunction {
  const tracker = options.tracker ?? serverConnections;
  const random = options.random ?? Math.random;

  return (operation, variables, cacheConfig, uploadables) =>
    Observable.create<GraphQLResponse>((sink) => {
      // connection は、送り直し始めてから、通るか諦めるまでの間だけ持つ。
      let connection: ReturnType<ConnectionTracker['track']> | undefined;
      let timer: ReturnType<typeof setTimeout> | undefined;
      let subscription: { unsubscribe: () => void } | undefined;
      let disposed = false;
      let retries = 0;

      const start = () => {
        subscription = Observable.from(
          fetchFn(operation, variables, cacheConfig, uploadables),
        ).subscribe({
          next: (value) => {
            sink.next(value);
          },
          complete: () => {
            connection?.connected();
            connection?.release();
            sink.complete();
          },
          error: (error: Error) => {
            if (disposed) {
              return;
            }
            if (
              operation.operationKind !== 'query' ||
              retries >= maxRetries ||
              !isTransientError(error)
            ) {
              connection?.release();
              sink.error(error);

              return;
            }
            if (connection === undefined) {
              connection = tracker.track();
              connection.reconnecting();
            }
            timer = setTimeout(start, retryDelayMs(retries, random()));
            retries++;
          },
        });
      };
      start();

      return () => {
        disposed = true;
        clearTimeout(timer);
        connection?.release();
        subscription?.unsubscribe();
      };
    });
}
