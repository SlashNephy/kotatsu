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

// 静的なデモ (#832) のエントリーポイント。`vp build --mode demo` のときだけ main.tsx が読み込む。
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
// 戻す関数を返す。Storybook (#2056) は、Story を閉じるときに戻す。
// Story は同時にいくつも開き (Docs のページ)、開いた順と逆に閉じるとは限らないので、横取りは 1 つだけ入れ、
// 最後に入れた応答の組 (interceptors の末尾) で応える。組がなくなったら、元の fetch に戻す。
// auth は、/internal/auth/* のパスごとに、デモの既定の応答の代わりに返す応答 (Storybook の Story で差し替える)。
type Interceptor = { executor: Executor; captures: DemoCaptures; auth: Record<string, unknown> };
const interceptors: Interceptor[] = [];
let baseFetch: typeof window.fetch | null = null;

export function interceptFetch(
  executor: Executor,
  captures: DemoCaptures,
  auth: Record<string, unknown> = {},
): () => void {
  const entry: Interceptor = { executor, captures, auth };
  interceptors.push(entry);
  if (baseFetch === null) {
    const base = window.fetch;
    baseFetch = base;
    const original = base.bind(window);
    window.fetch = async (input, init) => {
      const current = interceptors.at(-1);
      // 横取りしない要求の body を読んでしまわないよう、URL だけを先に見る。
      const url = new URL(input instanceof Request ? input.url : input, window.location.href);
      if (current === undefined || url.origin !== window.location.origin) {
        return original(input, init);
      }
      if (url.pathname === '/internal/graphql') {
        const request = new Request(input, init);
        const body = (await request.json()) as GraphQLBody;

        return json(await current.executor.execute(body.query ?? '', body.variables ?? {}));
      }
      if (url.pathname.startsWith('/internal/auth/')) {
        const replaced = current.auth[url.pathname];
        // Error を渡したパスは、失敗の応答を返す (失敗の通知を見るため)。
        if (replaced instanceof Error) {
          return new Response(JSON.stringify({ error: replaced.message }), {
            status: 500,
            headers: { 'Content-Type': 'application/json' },
          });
        }

        return json(replaced ?? authResponse(url.pathname));
      }
      // 録画の再生で読む実況の過去ログ。どの録画も同じ映像を流すので、同じコメントを返す。
      if (url.pathname === demoCommentsPath) {
        return json({ channel: { id: 'demo' }, origin: null, comments: demoRecordedComments() });
      }
      if (url.pathname === '/internal/media/captures') {
        const id = await current.captures.add(await new Request(input, init).formData());

        return json({ id });
      }

      return original(input, init);
    };
  }

  return () => {
    const index = interceptors.indexOf(entry);
    if (index >= 0) {
      interceptors.splice(index, 1);
    }
    if (interceptors.length === 0 && baseFetch !== null) {
      window.fetch = baseFetch;
      baseFetch = null;
    }
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
// Storybook の画面の Story (#2056) も、映像を流すのに使う。
export function registerStreamWorker(): void {
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

// DemoRoot は、デモの応答のフィールド。関数なら、引数を受け取って値を返す。
export type DemoRoot = Record<string, unknown>;

// DemoEnvironmentOptions は、createDemoEnvironment の設定。overrides のフィールドは、デモの値より優先する。
// now は、デモの番組表を作る時刻 (ミリ秒)。hold が true を返した操作には、応答を返さず待たせる。
export type DemoEnvironmentOptions = {
  // overrides は、デモの応答のうち差し替えるフィールド。関数なら、デモの既定の応答を受け取り、差し替えるフィールドを返す。
  // 既定の応答の一部だけを変える (予約の割り当てを競合にするなど) ときに使う。
  overrides?: object | ((root: DemoRoot) => object);
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
}: DemoEnvironmentOptions = {}): {
  environment: Environment;
  executor: Executor;
  captures: DemoCaptures;
} {
  const root = createRootValue(createEventHub(), now, captures) as DemoRoot;
  const executor = createExecutor({
    ...root,
    ...(typeof overrides === 'function' ? overrides(root) : overrides),
  });

  return {
    environment: new Environment({
      network: createNetwork(executor, hold),
      store: new Store(new RecordSource()),
    }),
    executor,
    captures,
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
