import { createClient, type Client } from 'graphql-sse';
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

import { retryDelayMs, serverConnections } from './connection';
import { GraphQLRequestError } from './requestError';
import { withTransientRetry } from './withTransientRetry';

export { GraphQLRequestError } from './requestError';

// GraphQL のエンドポイント (設計書 9.1)。tenban と同じオリジンに置く。
const endpoint = '/graphql';

// isUnauthenticated は、ログインしていないために拒否されたエラーかどうかを返す。
export function isUnauthenticated(error: unknown): boolean {
  return error instanceof GraphQLRequestError && error.code === 'UNAUTHENTICATED';
}

// GraphQLErrorDetails は、GraphQL のエラーの extensions のうち、画面が使うもの。
export type GraphQLErrorDetails = {
  // code は、エラーの種類 (REAUTHENTICATION_REQUIRED、BAD_USER_INPUT など)。
  code: string | undefined;
  // reason は、パスワードの規定に当たったときの理由 (too_weak など)。
  reason: string | undefined;
};

type RelaySourceError = {
  source?: { errors?: { extensions?: { code?: unknown; reason?: unknown } }[] | null };
};

// graphQLErrorDetails は、query や mutation が失敗したエラーから、最初の GraphQL のエラーの extensions を取り出す。
// Relay は、data のない応答を、source.errors に GraphQL のエラーを持つ Error にして投げる。
export function graphQLErrorDetails(error: unknown): GraphQLErrorDetails {
  if (error instanceof GraphQLRequestError) {
    return { code: error.code, reason: undefined };
  }
  const extensions = (error as RelaySourceError | null)?.source?.errors?.[0]?.extensions;
  const text = (value: unknown) => (typeof value === 'string' ? value : undefined);

  return { code: text(extensions?.code), reason: text(extensions?.reason) };
}

type ErrorBody = {
  errors?: { message?: string; extensions?: { code?: string } }[];
};

// fetchQuery は、query と mutation を POST の JSON で送る。
// tenban は、CSRF を防ぐため、GET と JSON でない POST を受け付けない (設計書 9.5)。
// GraphQL の処理の中で起きたエラーは 200 で返り、Relay がエラーとして扱う。
// それより前に拒否されたもの (401 など) と、JSON でない応答は、エラーコードを持つ GraphQLRequestError にする。
export const fetchQuery: FetchFunction = async (
  operation,
  variables,
  _cacheConfig,
  _uploadables,
) => {
  const response = await fetch(endpoint, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({ query: operation.text, variables }),
  });
  if (!(response.headers.get('Content-Type') ?? '').startsWith('application/json')) {
    throw new GraphQLRequestError(
      `unexpected response (HTTP ${response.status})`,
      response.status,
      undefined,
    );
  }
  const body: unknown = await response.json();
  if (!response.ok) {
    const first = (body as ErrorBody).errors?.[0];
    throw new GraphQLRequestError(
      first?.message ?? `HTTP ${response.status}`,
      response.status,
      first?.extensions?.code,
    );
  }

  return body as GraphQLResponse;
};

let sseClient: Client | undefined;

// subscribe は、subscription を graphql-sse で受け取る。tenban は、同じエンドポイントで SSE を返す。
// 接続が切れたら、上限なしにつなぎ直す。graphql-sse の既定 (5 回、約 30 秒) で諦めると、
// サーバーの再起動などの後、ページを読み込み直すまで変更が届かなくなるため (#503)。
// つなぎ直している間は、serverConnections に知らせ、ヘッダーに印を出す。
const subscribe: SubscribeFunction = (operation, variables) => {
  sseClient ??= createClient({
    url: endpoint,
    credentials: 'same-origin',
    retryAttempts: Infinity,
    retry: (retries) =>
      new Promise((resolve) => setTimeout(resolve, retryDelayMs(retries, Math.random()))),
  });
  const client = sseClient;

  return Observable.create<GraphQLResponse>((sink) => {
    const connection = serverConnections.track();
    const dispose = client.subscribe(
      { query: operation.text ?? '', variables },
      {
        next: (value) => {
          sink.next(value as GraphQLResponse);
        },
        error: (error) => {
          sink.error(error instanceof Error ? error : new Error(String(error)));
        },
        complete: () => {
          sink.complete();
        },
      },
      {
        connecting: (reconnecting) => {
          if (reconnecting) {
            connection.reconnecting();
          }
        },
        connected: () => connection.connected(),
      },
    );

    return () => {
      connection.release();
      dispose();
    };
  });
};

export function createEnvironment(): Environment {
  return new Environment({
    network: Network.create(withTransientRetry(fetchQuery), subscribe),
    store: new Store(new RecordSource()),
  });
}
