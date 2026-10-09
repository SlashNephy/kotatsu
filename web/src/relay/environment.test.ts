import type { RequestParameters } from 'relay-runtime';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { fetchQuery, GraphQLRequestError, isUnauthenticated } from './environment';

const operation = {
  text: 'query viewer { viewer { id } }',
  name: 'viewer',
  operationKind: 'query',
} as RequestParameters;

function respond(status: number, contentType: string, body: string) {
  const fetchMock = vi.fn(
    async () => new Response(body, { status, headers: { 'Content-Type': contentType } }),
  );
  vi.stubGlobal('fetch', fetchMock);

  return fetchMock;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchQuery', () => {
  test('POST の JSON で送り、Cookie を付け、応答をそのまま返す', async () => {
    const fetchMock = respond(200, 'application/json', '{"data":{"viewer":{"id":"VXNlcjox"}}}');

    await expect(fetchQuery(operation, { first: 1 }, {}, undefined)).resolves.toEqual({
      data: { viewer: { id: 'VXNlcjox' } },
    });
    expect(fetchMock).toHaveBeenCalledWith('/graphql', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ query: operation.text, variables: { first: 1 } }),
    });
  });

  test.each([
    {
      name: 'ログインしていない',
      status: 401,
      contentType: 'application/json',
      body: '{"errors":[{"message":"login required","extensions":{"code":"UNAUTHENTICATED"}}]}',
      want: {
        message: 'login required',
        status: 401,
        code: 'UNAUTHENTICATED',
        unauthenticated: true,
      },
    },
    {
      name: 'JSON でない、リバースプロキシのエラー',
      status: 502,
      contentType: 'text/html',
      body: '<html>Bad Gateway</html>',
      want: {
        message: 'unexpected response (HTTP 502)',
        status: 502,
        code: undefined,
        unauthenticated: false,
      },
    },
    {
      name: 'エラーコードのない拒否',
      status: 415,
      contentType: 'application/json',
      body: '{}',
      want: { message: 'HTTP 415', status: 415, code: undefined, unauthenticated: false },
    },
  ])('$name応答は、GraphQLRequestError にする', async ({ status, contentType, body, want }) => {
    respond(status, contentType, body);

    const error: unknown = await Promise.resolve(fetchQuery(operation, {}, {}, undefined)).catch(
      (e: unknown) => e,
    );
    expect(error).toBeInstanceOf(GraphQLRequestError);
    expect(error).toMatchObject({ message: want.message, status: want.status, code: want.code });
    expect(isUnauthenticated(error)).toBe(want.unauthenticated);
  });
});
