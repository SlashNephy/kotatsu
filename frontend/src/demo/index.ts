import {
  Environment,
  Network,
  Observable,
  RecordSource,
  Store,
  type FetchFunction,
  type GraphQLResponse,
  type SubscribeFunction,
} from 'relay-runtime';

import { authResponse } from './auth';
import { createRootValue } from './data';
import { createEventHub } from './events';
import { createExecutor, type Executor, type Variables } from './executor';
import { demoCommentsPath, demoRecordedComments, installFakeJikkyo } from './jikkyo';

// 静的なデモ (#832) の入口。`vp build --mode demo` のときだけ main.tsx が読み込む。
// tenban の代わりに、GraphQL と /auth/* の応答をブラウザの中で返す。

type GraphQLBody = { query?: string; variables?: Variables | null };

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}

// interceptFetch は、Relay を通らずに /graphql と /auth/* を呼ぶ箇所 (タイムゾーンの読み込み、
// 視聴位置の keepalive の保存、ログインやログアウト) と、実況の過去ログ (src/demo/jikkyo.ts) に、デモの応答を返す。
// 呼ぶ側はサブパスを付けずに / からのパスで呼ぶので、同じオリジンのそのパスだけを横取りする。
function interceptFetch(executor: Executor): void {
  const original = window.fetch.bind(window);
  window.fetch = async (input, init) => {
    // 横取りしない要求の body を読んでしまわないよう、URL だけを先に見る。
    const url = new URL(input instanceof Request ? input.url : input, window.location.href);
    if (url.origin !== window.location.origin) {
      return original(input, init);
    }
    if (url.pathname === '/graphql') {
      const request = new Request(input, init);
      const body = (await request.json()) as GraphQLBody;

      return json(await executor.execute(body.query ?? '', body.variables ?? {}));
    }
    if (url.pathname.startsWith('/auth/')) {
      return json(authResponse(url.pathname));
    }
    // 録画の再生で読む実況の過去ログ。どの録画も同じ映像を流すので、同じコメントを返す。
    if (url.pathname === demoCommentsPath) {
      return json({ channel: { id: 'demo' }, origin: null, comments: demoRecordedComments() });
    }

    return original(input, init);
  };
}

function createNetwork(executor: Executor) {
  const fetchQuery: FetchFunction = async (operation, variables) =>
    (await executor.execute(operation.text ?? '', variables)) as GraphQLResponse;

  const subscribe: SubscribeFunction = (operation, variables) =>
    Observable.create<GraphQLResponse>((sink) => {
      let stopped = false;
      let iterator: AsyncIterator<unknown> | undefined;
      void (async () => {
        try {
          const results = await executor.subscribe(operation.text ?? '', variables);
          iterator = results[Symbol.asyncIterator]();
          if (stopped) {
            await iterator.return?.();
            return;
          }
          for (;;) {
            const next = await iterator.next();
            if (next.done === true || stopped) {
              break;
            }
            sink.next(next.value as GraphQLResponse);
          }
          sink.complete();
        } catch (error) {
          sink.error(error instanceof Error ? error : new Error(String(error)));
        }
      })();

      return () => {
        stopped = true;
        void iterator?.return?.();
      };
    });

  return Network.create(fetchQuery, subscribe);
}

// registerStreamWorker は、ライブ視聴の映像を実時間の速さで流す Service Worker (demo/stream-sw.js) を登録する。
// 登録を待たずに画面を出す。Service Worker は登録した直後のページも受け持つので、ライブ視聴を開くまでには間に合う。
function registerStreamWorker(): void {
  if (!('serviceWorker' in navigator)) {
    return;
  }
  const base = import.meta.env.BASE_URL;
  // 登録できなくても、ライブ視聴が映らないだけなので、画面はそのまま出す。
  navigator.serviceWorker
    .register(`${base}stream-sw.js`, { scope: base })
    .catch((error: unknown) => {
      console.warn('failed to register the demo stream worker', error);
    });
}

// installDemo は、fetch と実況の WebSocket を横取りし、デモの応答を返す Relay の Environment を作る。
export function installDemo(): Environment {
  const executor = createExecutor(createRootValue(createEventHub()));
  interceptFetch(executor);
  installFakeJikkyo();
  registerStreamWorker();

  return new Environment({
    network: createNetwork(executor),
    store: new Store(new RecordSource()),
  });
}
