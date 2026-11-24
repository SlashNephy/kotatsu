import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { commitLocalUpdate } from 'relay-runtime';
import { afterEach, describe, expect, test } from 'vitest';

import { createTestEnvironment, renderRoute } from '../../test/route';
import { Route } from './services';

afterEach(cleanup);

// servicesResponse は、サービスが 1 つの画面の応答を作る。hidden は、そのサービスの番組表に出さないマーク、sortOrder は並び順の上書き。
// jikkyoChannels は、画面で足した実況のチャンネルの対応 (#1869)。
function servicesResponse(
  hidden: boolean,
  sortOrder: number | null = null,
  jikkyoChannels: { networkId: number; serviceId: number; channelId: string }[] = [],
) {
  return {
    data: {
      channelTypes: [],
      services: [
        {
          id: 'U2VydmljZTox',
          networkId: 32736,
          serviceId: 1024,
          name: 'こたつテスト1',
          logoUrl: null,
          hidden,
          sortOrder,
          channelType: { displayName: '地上波' },
        },
      ],
      systemSettings: { animeLinkingEnabled: false, jikkyoChannels },
      animeChannels: [],
    },
  };
}

describe('番組表に出さない印は、変えたらすぐ保存する (#1740)', () => {
  test('印だけを送り、書きかけの並び順は送らずに残す', async () => {
    let hidden = false;
    const sent: unknown[] = [];
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'servicesServiceMutation') {
        sent.push(variables.input);
        hidden = (variables.input as { hidden: boolean }).hidden;

        return {
          data: { updateService: { id: 'U2VydmljZTox', hidden, sortOrder: null } },
        };
      }

      return servicesResponse(hidden);
    });
    await renderRoute(Route, '/admin/services', '/admin/services', environment);
    fireEvent.change(await screen.findByRole('textbox', { name: '並び順' }), {
      target: { value: '5' },
    });

    await act(async () => fireEvent.click(screen.getByRole('switch', { name: '出さない' })));
    await screen.findByRole('status');

    expect(sent).toEqual([{ hidden: true }]);
    expect(screen.getByRole('switch', { name: '出さない' })).toHaveProperty('checked', true);
    expect(screen.getByRole('textbox', { name: '並び順' })).toHaveProperty('value', '5');
  });

  test('保存に失敗したら、変える前に戻す', async () => {
    const environment = createTestEnvironment((request) =>
      request.name === 'servicesServiceMutation'
        ? new Error('unavailable')
        : servicesResponse(false),
    );
    await renderRoute(Route, '/admin/services', '/admin/services', environment);
    const toggle = await screen.findByRole('switch', { name: '出さない' });

    await act(async () => fireEvent.click(toggle));
    await screen.findByRole('alert');

    expect(screen.getByRole('switch', { name: '出さない' })).toHaveProperty('checked', false);
  });

  test('保存した後に、ほかの管理者が戻した値を読み直したら、その値を出す', async () => {
    let hidden = false;
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'servicesServiceMutation') {
        hidden = (variables.input as { hidden: boolean }).hidden;

        return {
          data: { updateService: { id: 'U2VydmljZTox', hidden, sortOrder: null } },
        };
      }

      return servicesResponse(hidden);
    });
    await renderRoute(Route, '/admin/services', '/admin/services', environment);
    await act(async () => fireEvent.click(await screen.findByRole('switch', { name: '出さない' })));
    await screen.findByRole('status');

    act(() =>
      commitLocalUpdate(environment, (store) => {
        store.get('U2VydmljZTox')?.setValue(false, 'hidden');
      }),
    );

    expect(screen.getByRole('switch', { name: '出さない' })).toHaveProperty('checked', false);
  });
});

describe('並び順は、欄を離れたときか Enter で保存する (#1740)', () => {
  test.each([
    { name: '数を入れたら、その並び順を送る', value: '5', want: { sortOrder: 5 } },
    { name: '空にしたら、上書きを消す', value: '', want: { resetSortOrder: true } },
  ])('$name', async ({ value, want }) => {
    const sent: unknown[] = [];
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'servicesServiceMutation') {
        sent.push(variables.input);

        return { data: { updateService: { id: 'U2VydmljZTox', hidden: false, sortOrder: 3 } } };
      }
      return servicesResponse(false, 3);
    });
    await renderRoute(Route, '/admin/services', '/admin/services', environment);
    const order = await screen.findByRole('textbox', { name: '並び順' });
    fireEvent.change(order, { target: { value } });
    expect(sent).toEqual([]);

    await act(async () => fireEvent.blur(order));
    await screen.findByRole('status');

    expect(sent).toEqual([want]);
  });
});

describe('ニコニコ実況のチャンネル (#1869)', () => {
  test.each([
    { name: '入れたチャンネルを、サービスの組で送る', saved: [], value: 'jk8', want: 'jk8' },
    {
      name: '空にすると、対応を消す (null を送る)',
      saved: [{ networkId: 32736, serviceId: 1024, channelId: 'jk8' }],
      value: '',
      want: null,
    },
  ])('$name', async ({ saved, value, want }) => {
    const sent: unknown[] = [];
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'servicesJikkyoChannelMutation') {
        sent.push(variables.input);

        return { data: { updateSystemSettings: { jikkyoChannels: [] } } };
      }

      return servicesResponse(false, null, saved);
    });
    await renderRoute(Route, '/admin/services', '/admin/services', environment);
    const channel = await screen.findByRole('textbox', { name: '実況のチャンネル（jk1など）' });
    fireEvent.change(channel, { target: { value } });

    await act(async () => fireEvent.blur(channel));
    await screen.findByRole('status');

    expect(sent).toEqual([
      { jikkyoChannels: [{ networkId: 32736, serviceId: 1024, channelId: want }] },
    ]);
  });

  test('jk と数字でなければ、理由を出して保存しない', async () => {
    const sent: unknown[] = [];
    const environment = createTestEnvironment((request, variables) => {
      if (request.name === 'servicesJikkyoChannelMutation') {
        sent.push(variables.input);
      }

      return servicesResponse(false);
    });
    await renderRoute(Route, '/admin/services', '/admin/services', environment);
    const channel = await screen.findByRole('textbox', { name: '実況のチャンネル（jk1など）' });
    fireEvent.change(channel, { target: { value: '101' } });

    await act(async () => fireEvent.blur(channel));

    expect(sent).toEqual([]);
    expect(channel.getAttribute('aria-invalid')).toBe('true');
  });
});
