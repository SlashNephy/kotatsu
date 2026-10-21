import {
  Observable,
  type FetchFunction,
  type GraphQLResponse,
  type RequestParameters,
} from 'relay-runtime';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { createConnectionTracker } from './connection';
import { GraphQLRequestError } from './requestError';
import { maxRetries, withTransientRetry } from './withTransientRetry';

function operation(operationKind: 'query' | 'mutation'): RequestParameters {
  return { text: '', name: 'test', operationKind } as RequestParameters;
}

const ok: GraphQLResponse = { data: { viewer: null } };
const unavailable = () => new GraphQLRequestError('unexpected response (HTTP 503)', 503, undefined);

// fetcher は、results を順に返す FetchFunction と、呼ばれた回数を返す。
function fetcher(results: Array<GraphQLResponse | Error>) {
  const fetchFn = vi.fn<FetchFunction>(() => {
    const result = results.shift();
    if (result === undefined) {
      throw new Error('no more results');
    }

    return Observable.create((sink) => {
      if (result instanceof Error) {
        sink.error(result);
      } else {
        sink.next(result);
        sink.complete();
      }
    });
  });

  return fetchFn;
}

// run は、FetchFunction を購読し、届いた値とエラーを集める。
function run(fetchFn: FetchFunction, kind: 'query' | 'mutation' = 'query') {
  const got: { values: GraphQLResponse[]; error: Error | undefined; completed: boolean } = {
    values: [],
    error: undefined,
    completed: false,
  };
  const subscription = Observable.from(fetchFn(operation(kind), {}, {}, undefined)).subscribe({
    next: (value) => got.values.push(value),
    error: (error: Error) => {
      got.error = error;
    },
    complete: () => {
      got.completed = true;
    },
  });

  return { got, subscription };
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('withTransientRetry', () => {
  test('一時的な障害で失敗した query は、間をおいて送り直し、通ればエラーを出さずに応答を返す', async () => {
    const tracker = createConnectionTracker();
    const fetchFn = fetcher([unavailable(), unavailable(), ok]);
    const { got } = run(withTransientRetry(fetchFn, { tracker, random: () => 0 }));

    // 送り直している間は、つなぎ直している接続として数え、エラーを出さない。
    expect(got.error).toBeUndefined();
    expect(tracker.isReconnecting()).toBe(true);

    await vi.runAllTimersAsync();

    expect(fetchFn).toHaveBeenCalledTimes(3);
    expect(got).toEqual({ values: [ok], error: undefined, completed: true });
    expect(tracker.isReconnecting()).toBe(false);
    // 送り直して通ったので、サーバーが戻ったと知らせる。
    expect(tracker.recoveries()).toBe(1);
  });

  test('一度で通った query は、サーバーが戻ったとは知らせない', () => {
    const tracker = createConnectionTracker();
    const { got } = run(withTransientRetry(fetcher([ok]), { tracker }));

    expect(got.completed).toBe(true);
    expect(tracker.recoveries()).toBe(0);
  });

  test('送り直す回数の上限を使い切ったら、最後のエラーを返し、つなぎ直している接続から外す', async () => {
    const tracker = createConnectionTracker();
    const last = unavailable();
    const fetchFn = fetcher([...Array.from({ length: maxRetries }, unavailable), last]);
    const { got } = run(withTransientRetry(fetchFn, { tracker, random: () => 0 }));

    await vi.runAllTimersAsync();

    expect(fetchFn).toHaveBeenCalledTimes(maxRetries + 1);
    expect(got.error).toBe(last);
    expect(tracker.isReconnecting()).toBe(false);
    expect(tracker.recoveries()).toBe(0);
  });

  test.each([
    {
      name: '送り直しても変わらないエラー',
      kind: 'query' as const,
      error: new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED'),
    },
    {
      name: 'mutation (同じ操作を 2 回行わないため)',
      kind: 'mutation' as const,
      error: unavailable(),
    },
  ])('$name は、送り直さずにエラーを返す', ({ kind, error }) => {
    const tracker = createConnectionTracker();
    const fetchFn = fetcher([error]);
    const { got } = run(withTransientRetry(fetchFn, { tracker }), kind);

    expect(fetchFn).toHaveBeenCalledTimes(1);
    expect(got.error).toBe(error);
    expect(tracker.isReconnecting()).toBe(false);
  });

  test('送り直すのを待っている間にやめたら、送り直さず、つなぎ直している接続から外す', async () => {
    const tracker = createConnectionTracker();
    const fetchFn = fetcher([unavailable(), ok]);
    const { got, subscription } = run(withTransientRetry(fetchFn, { tracker }));

    subscription.unsubscribe();
    await vi.runAllTimersAsync();

    expect(fetchFn).toHaveBeenCalledTimes(1);
    expect(got.values).toEqual([]);
    expect(tracker.isReconnecting()).toBe(false);
  });
});
