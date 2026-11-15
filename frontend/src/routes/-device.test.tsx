import { act, cleanup, fireEvent, screen, waitFor } from '@testing-library/react';
import type { GraphQLResponse, RequestParameters, Variables } from 'relay-runtime';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { authApi } from '../auth/api';
import { normalizeUserCode } from '../auth/userCode';
import { GraphQLRequestError } from '../relay/requestError';
import { createTestEnvironment, renderRoute } from '../test/route';
import { Route } from './device';

type DeviceAuthorization = { deviceName: string; userCode: string; approved: boolean } | null;

// deviceServer は、承認の画面が送る GraphQL の操作に応える。sent は、送られた操作の名前と変数。
// device は、引いた user_code の端末を返す。conflict なら、承認にほかのユーザーが承認済みと答える。
function deviceServer(
  device: DeviceAuthorization | ((userCode: string) => DeviceAuthorization),
  { conflict = false }: { conflict?: boolean } = {},
) {
  const sent: { name: string; variables: Variables }[] = [];
  const environment = createTestEnvironment(
    (request: RequestParameters, variables: Variables): GraphQLResponse | Error => {
      sent.push({ name: request.name, variables });
      if (request.name === 'deviceApproveMutation') {
        return conflict
          ? new GraphQLRequestError('approved by another user', 200, 'CONFLICT')
          : { data: { approveDeviceAuthorization: { approved: true } } };
      }

      return {
        data: {
          deviceAuthorization:
            typeof device === 'function' ? device(String(variables.userCode)) : device,
          secondFactors: { totp: false, remainingRecoveryCodes: 0 },
          passkeys: [],
        },
      };
    },
  );

  return { environment, sent };
}

const livingRoomTV: DeviceAuthorization = {
  deviceName: 'リビングのテレビ',
  userCode: 'ABCD-EFGH',
  approved: false,
};

beforeEach(() => {
  vi.spyOn(authApi, 'oidcStatus').mockResolvedValue({
    enabled: false,
    displayName: '',
    linked: false,
  });
  vi.spyOn(authApi, 'reauthenticationStatus').mockResolvedValue({
    authenticatedAt: new Date().toISOString(),
    recent: true,
  });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe('承認の画面 (#1527)', () => {
  test('コード付きの URL を開いただけでは承認せず、端末の名前とコードを出す。ボタンを押したら承認する', async () => {
    const { environment, sent } = deviceServer(livingRoomTV);
    await renderRoute(Route, '/device', '/device?user_code=abcd-efgh', environment);

    expect(await screen.findByText('リビングのテレビ')).toBeTruthy();
    expect(screen.getByText('ABCD-EFGH')).toBeTruthy();
    // 打ち方の揺れは、正規化してから引く。
    expect(sent).toEqual([{ name: 'deviceQuery', variables: { userCode: 'ABCD-EFGH' } }]);

    fireEvent.click(await screen.findByRole('button', { name: '承認する' }));

    expect(await screen.findByText(/承認しました/)).toBeTruthy();
    expect(sent.map(({ name }) => name)).toEqual(['deviceQuery', 'deviceApproveMutation']);
    expect(sent[1]?.variables).toEqual({ userCode: 'ABCD-EFGH' });
  });

  test('名前を名乗らない端末は、既定の名前で出す', async () => {
    const { environment } = deviceServer({ ...livingRoomTV!, deviceName: '' });
    await renderRoute(Route, '/device', '/device?user_code=ABCD-EFGH', environment);

    expect(await screen.findByText('名前のない端末')).toBeTruthy();
  });

  test('有効なコードがなければ、承認のボタンを出さずに、入れ直させる', async () => {
    const { environment } = deviceServer(null);
    await renderRoute(Route, '/device', '/device?user_code=ABCD-EFGH', environment);

    expect(await screen.findByText(/コード ABCD-EFGH の端末が見つかりません/)).toBeTruthy();
    expect(screen.queryByRole('button', { name: '承認する' })).toBeNull();
  });

  test('コードを手で打つときは、正しい形になるまで引かない', async () => {
    const { environment, sent } = deviceServer(livingRoomTV);
    await renderRoute(Route, '/device', '/device', environment);

    const input = await screen.findByRole('textbox', { name: 'コード' });
    fireEvent.change(input, { target: { value: 'ABCD-EFG0' } });
    fireEvent.click(screen.getByRole('button', { name: '次へ' }));
    expect(await screen.findByText(/英字と数字の8字/)).toBeTruthy();
    expect(sent).toEqual([]);

    fireEvent.change(input, { target: { value: 'ａｂｃｄ ｅｆｇｈ' } });
    fireEvent.click(screen.getByRole('button', { name: '次へ' }));

    expect(await screen.findByText('リビングのテレビ')).toBeTruthy();
    await waitFor(() =>
      expect(sent).toEqual([{ name: 'deviceQuery', variables: { userCode: 'ABCD-EFGH' } }]),
    );
  });
});

describe('承認の画面の、承認済みと断られたとき (#1527)', () => {
  test('ほかのユーザーが承認済みなら、そう伝える', async () => {
    const { environment } = deviceServer(livingRoomTV, { conflict: true });
    await renderRoute(Route, '/device', '/device?user_code=ABCD-EFGH', environment);

    fireEvent.click(await screen.findByRole('button', { name: '承認する' }));

    expect(await screen.findByText(/ほかのユーザーが承認済み/)).toBeTruthy();
  });

  test('承認した後に別のコードへ移ったら、承認済みの表示を残さない', async () => {
    const { environment } = deviceServer((userCode) => ({
      deviceName: userCode === 'ABCD-EFGH' ? 'リビングのテレビ' : '寝室のテレビ',
      userCode,
      approved: false,
    }));
    const { router } = await renderRoute(
      Route,
      '/device',
      '/device?user_code=ABCD-EFGH',
      environment,
    );
    fireEvent.click(await screen.findByRole('button', { name: '承認する' }));
    expect(await screen.findByText(/承認しました/)).toBeTruthy();

    await act(() => router.navigate({ to: '/device', search: { user_code: 'JKLM-NPQR' } }));

    expect(await screen.findByText('寝室のテレビ')).toBeTruthy();
    expect(screen.queryByText(/承認しました/)).toBeNull();
    expect(await screen.findByRole('button', { name: '承認する' })).toBeTruthy();
  });
});

describe('normalizeUserCode', () => {
  test.each([
    ['ABCD-EFGH', 'ABCD-EFGH'],
    ['abcdefgh', 'ABCD-EFGH'],
    [' AB CD--EF GH ', 'ABCD-EFGH'],
    ['ＡＢＣＤ－ＥＦＧＨ', 'ABCD-EFGH'],
    ['ABCD-EFG0', null],
    ['ABCD-EFGI', null],
    ['ABCD-EFG', null],
    ['ABCD-EFGHJ', null],
  ])('%s は %s', (input, want) => {
    expect(normalizeUserCode(input)).toBe(want);
  });
});
