import { Environment, Network, RecordSource, Store, type GraphQLResponse } from 'relay-runtime';
import { describe, expect, test } from 'vitest';

import { AuthError } from '../auth/api';
import securityRegenerateMutation, {
  type securityRegenerateMutation as Mutation,
} from '../routes/settings/__generated__/securityRegenerateMutation.graphql';
import { mutate } from './mutate';
import { needsReauthentication } from './useReauthGate';

// environment は、どの mutation にも payload を返す Relay の環境を作る。
function environment(payload: GraphQLResponse) {
  return new Environment({
    network: Network.create(async () => payload),
    store: new Store(new RecordSource()),
  });
}

const reauthenticationRequired = {
  message: 'reauthentication required',
  extensions: { code: 'REAUTHENTICATION_REQUIRED' },
};

describe('mutate', () => {
  test('エラーがなければ、応答のデータで成功する', async () => {
    const codes = ['aaaaa-bbbbb', 'ccccc-ddddd'];

    await expect(
      mutate<Mutation>(
        environment({ data: { regenerateRecoveryCodes: { codes } } }),
        securityRegenerateMutation,
        {},
      ),
    ).resolves.toEqual({ regenerateRecoveryCodes: { codes } });
  });

  test.each([
    {
      name: 'データとともにエラーが返ったとき',
      payload: { data: { regenerateRecoveryCodes: null }, errors: [reauthenticationRequired] },
      reauthenticate: true,
    },
    {
      name: 'データなしでエラーが返ったとき',
      payload: { data: null, errors: [reauthenticationRequired] },
      reauthenticate: true,
    },
    {
      name: '再認証と関係のないエラーのとき',
      payload: {
        data: null,
        errors: [{ message: 'forbidden', extensions: { code: 'FORBIDDEN' } }],
      },
      reauthenticate: false,
    },
  ])('$name は失敗し、再認証が要るかをエラーから読める', async ({ payload, reauthenticate }) => {
    const error: unknown = await mutate<Mutation>(
      environment(payload as GraphQLResponse),
      securityRegenerateMutation,
      {},
    ).then(
      () => undefined,
      (caught: unknown) => caught,
    );

    expect(error).toBeInstanceOf(Error);
    expect(needsReauthentication(error)).toBe(reauthenticate);
  });
});

describe('needsReauthentication', () => {
  test.each([
    {
      name: '認証の API が再認証を求めたとき',
      error: new AuthError(403, 'REAUTHENTICATION_REQUIRED', undefined, undefined),
      want: true,
    },
    {
      name: '認証の API がほかの理由で断ったとき',
      error: new AuthError(401, 'INVALID_CREDENTIALS', undefined, undefined),
      want: false,
    },
    { name: '通信のエラー', error: new TypeError('Failed to fetch'), want: false },
  ])('$name', ({ error, want }) => {
    expect(needsReauthentication(error)).toBe(want);
  });
});
