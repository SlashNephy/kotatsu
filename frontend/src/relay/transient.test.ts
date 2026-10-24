import { describe, expect, test } from 'vitest';

import { GraphQLRequestError } from './requestError';
import { isTransientError } from './transient';

describe('isTransientError', () => {
  test.each([
    {
      name: 'リバースプロキシの 503',
      error: new GraphQLRequestError('x', 503, undefined),
      want: true,
    },
    {
      name: 'リバースプロキシの 502',
      error: new GraphQLRequestError('x', 502, undefined),
      want: true,
    },
    { name: 'tenban の 500', error: new GraphQLRequestError('x', 500, undefined), want: true },
    { name: '429', error: new GraphQLRequestError('x', 429, undefined), want: true },
    { name: '408', error: new GraphQLRequestError('x', 408, undefined), want: true },
    {
      name: 'ログインしていない 401',
      error: new GraphQLRequestError('x', 401, 'UNAUTHENTICATED'),
      want: false,
    },
    { name: '403', error: new GraphQLRequestError('x', 403, 'FORBIDDEN'), want: false },
    { name: '404', error: new GraphQLRequestError('x', 404, undefined), want: false },
    { name: 'Chromium の届かなかった fetch', error: new TypeError('Failed to fetch'), want: true },
    {
      name: 'Firefox の届かなかった fetch',
      error: new TypeError('NetworkError when attempting to fetch resource.'),
      want: true,
    },
    { name: 'WebKit の届かなかった fetch', error: new TypeError('Load failed'), want: true },
    {
      name: '文言が前方で一致するだけの TypeError',
      error: new TypeError('Failed to fetch media file: 500'),
      want: false,
    },
    { name: 'GraphQL のエラー', error: new Error('Relay: query failed'), want: false },
  ])('$name は $want', ({ error, want }) => {
    expect(isTransientError(error)).toBe(want);
  });
});
