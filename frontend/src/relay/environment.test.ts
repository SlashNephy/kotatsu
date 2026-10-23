import type { RequestParameters } from 'relay-runtime';
import { afterEach, describe, expect, test, vi } from 'vitest';

import {
  fetchQuery,
  graphQLErrorDetails,
  GraphQLRequestError,
  isUnauthenticated,
} from './environment';

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

describe('graphQLErrorDetails', () => {
  // relayError は、Relay が data のない応答から作るエラーと同じ形のもの。
  function relayError(errors: unknown) {
    return Object.assign(new Error('No data returned'), { source: { errors } });
  }

  test.each([
    {
      name: 'Relay のエラーから、最初の GraphQL のエラーの code と reason を取り出す',
      error: relayError([
        { message: 'weak', extensions: { code: 'BAD_USER_INPUT', reason: 'too_weak' } },
        { message: 'other', extensions: { code: 'INTERNAL' } },
      ]),
      want: { code: 'BAD_USER_INPUT', reason: 'too_weak' },
    },
    {
      name: 'reason のないエラー',
      error: relayError([{ message: 'reauth', extensions: { code: 'REAUTHENTICATION_REQUIRED' } }]),
      want: { code: 'REAUTHENTICATION_REQUIRED', reason: undefined },
    },
    {
      name: 'GraphQL の処理に入る前に拒否されたエラー',
      error: new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED'),
      want: { code: 'UNAUTHENTICATED', reason: undefined },
    },
    {
      name: '文字列でない extensions は使わない',
      error: relayError([{ message: 'x', extensions: { code: 1, reason: null } }]),
      want: { code: undefined, reason: undefined },
    },
    {
      name: 'GraphQL のエラーでないもの',
      error: new TypeError('Failed to fetch'),
      want: { code: undefined, reason: undefined },
    },
    {
      name: 'errors が null',
      error: relayError(null),
      want: { code: undefined, reason: undefined },
    },
  ])('$name', ({ error, want }) => {
    expect(graphQLErrorDetails(error)).toEqual(want);
  });
});
