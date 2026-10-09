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

// fetchQuery は、query と mutation を POST の JSON で送る。
// tenban は、CSRF を防ぐため、GET と JSON でない POST を受け付けない (設計書 9.5)。
const fetchQuery: FetchFunction = async (operation, variables, _cacheConfig, _uploadables) => {
  const response = await fetch(endpoint, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({ query: operation.text, variables }),
  });

  return (await response.json()) as GraphQLResponse;
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
