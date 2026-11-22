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
import { createDemoCaptures, type DemoCaptures } from './captures';
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
// 視聴位置の keepalive の保存、ログインやログアウト)、キャプチャを残す POST /media/captures と、実況の過去ログ (src/demo/jikkyo.ts) に、
// デモの応答を返す。
// 呼ぶ側はサブパスを付けずに / からのパスで呼ぶので、同じオリジンのそのパスだけを横取りする。
function interceptFetch(executor: Executor, captures: DemoCaptures): void {
  const original = window.fetch.bind(window);
  window.fetch = async (input, init) => {
    // 横取りしない要求の body を読んでしまわないよう、URL だけを先に見る。
    const url = new URL(input instanceof Request ? input.url : input, window.location.href);
    if (url.origin !== window.location.origin) {
      return original(input, init);
    }
    if (url.pathname === '/internal/graphql') {
      const request = new Request(input, init);
      const body = (await request.json()) as GraphQLBody;

      return json(await executor.execute(body.query ?? '', body.variables ?? {}));
    }
    if (url.pathname.startsWith('/internal/auth/')) {
      return json(authResponse(url.pathname));
    }
    // 録画の再生で読む実況の過去ログ。どの録画も同じ映像を流すので、同じコメントを返す。
    if (url.pathname === demoCommentsPath) {
      return json({ channel: { id: 'demo' }, origin: null, comments: demoRecordedComments() });
    }
    if (url.pathname === '/internal/media/captures') {
      const id = await captures.add(await new Request(input, init).formData());

      return json({ id });
    }

    return original(input, init);
  };
}

// Hold は、応答を返さずに待たせる操作を選ぶ。読み込み中の画面を見るのに使う (Storybook、#2056)。
export type Hold = (operation: { name: string }, variables: Variables) => boolean;

function createNetwork(executor: Executor, hold?: Hold) {
  const fetchQuery: FetchFunction = (operation, variables) =>
    Observable.create<GraphQLResponse>((sink) => {
      if (hold?.(operation, variables) === true) {
        return;
      }
      executor.execute(operation.text ?? '', variables).then(
        (result) => {
          sink.next(result as GraphQLResponse);
          sink.complete();
        },
        (error: unknown) => sink.error(error instanceof Error ? error : new Error(String(error))),
      );
    });

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

// registerStreamWorker は、ライブ視聴と録画の再生で、デモの映像を繰り返して流す Service Worker (demo/stream-sw.js) を登録する。
// Service Worker は、ES モジュール (demo/stream-loop.js) を読むので、モジュールとして登録する。
// 登録を待たずに画面を出す。Service Worker は登録した直後のページも受け持つので、ライブ視聴を開くまでには間に合う。
function registerStreamWorker(): void {
  if (!('serviceWorker' in navigator)) {
    return;
  }
  const base = import.meta.env.BASE_URL;
  // 登録できなくても、ライブ視聴が映らないだけなので、画面はそのまま出す。
  navigator.serviceWorker
    .register(`${base}stream-sw.js`, { scope: base, type: 'module' })
    .catch((error: unknown) => {
      console.warn('failed to register the demo stream worker', error);
    });
}

// DemoEnvironmentOptions は、createDemoEnvironment の設定。overrides のフィールドは、デモの値より優先する。
// now は、デモの番組表を作る時刻 (ミリ秒)。hold が true を返した操作には、応答を返さず待たせる。
export type DemoEnvironmentOptions = {
  overrides?: object;
  // fillMissing なら、デモのデータにない non-null のフィールドを、型の既定の値で埋める (executor.ts)。
  fillMissing?: boolean;
  now?: number;
  hold?: Hold;
  captures?: DemoCaptures;
};

// createDemoEnvironment は、デモの応答を返す Relay の Environment と、応答を作る executor を作る。fetch などは横取りしない。
// Storybook のページの Story (#2056) は、Story ごとに応答を差し替えて使う。
export function createDemoEnvironment({
  overrides = {},
  now = Date.now(),
  hold,
  captures = createDemoCaptures(),
  fillMissing = false,
}: DemoEnvironmentOptions = {}): { environment: Environment; executor: Executor } {
  const executor = createExecutor(
    { ...createRootValue(createEventHub(), now, captures), ...overrides },
    { fillMissing },
  );

  return {
    environment: new Environment({
      network: createNetwork(executor, hold),
      store: new Store(new RecordSource()),
    }),
    executor,
  };
}

// installDemo は、fetch と実況の WebSocket を横取りし、デモの応答を返す Relay の Environment を作る。
export function installDemo(): Environment {
  const captures = createDemoCaptures();
  const { environment, executor } = createDemoEnvironment({ captures });
  interceptFetch(executor, captures);
  installFakeJikkyo();
  registerStreamWorker();

  return environment;
}
