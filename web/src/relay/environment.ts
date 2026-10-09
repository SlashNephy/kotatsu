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

// GraphQL のエンドポイント (設計書 9.1)。tenban と同じオリジンに置く。
const endpoint = '/graphql';

// GraphQLRequestError は、GraphQL の処理に入る前に tenban が拒否したリクエストや、
// リバースプロキシのエラーのように GraphQL の応答でないものを表す。
export class GraphQLRequestError extends Error {
  readonly status: number;
  // code は、tenban が付けたエラーコード (UNAUTHENTICATED など)。GraphQL の応答でなければ undefined。
  readonly code: string | undefined;

  constructor(message: string, status: number, code: string | undefined) {
    super(message);
    this.name = 'GraphQLRequestError';
    this.status = status;
    this.code = code;
  }
}

// isUnauthenticated は、ログインしていないために拒否されたエラーかどうかを返す。
export function isUnauthenticated(error: unknown): boolean {
  return error instanceof GraphQLRequestError && error.code === 'UNAUTHENTICATED';
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
const subscribe: SubscribeFunction = (operation, variables) => {
  sseClient ??= createClient({ url: endpoint, credentials: 'same-origin' });
  const client = sseClient;

  return Observable.create<GraphQLResponse>((sink) =>
    client.subscribe(
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
    ),
  );
};

export function createEnvironment(): Environment {
  return new Environment({
    network: Network.create(fetchQuery, subscribe),
    store: new Store(new RecordSource()),
  });
}
