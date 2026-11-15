import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { commitLocalUpdate } from 'relay-runtime';
import { afterEach, describe, expect, test } from 'vitest';

import { createTestEnvironment, renderRoute } from '../../test/route';
import { Route } from './services';

afterEach(cleanup);

// servicesResponse は、サービスが 1 つの画面の応答を作る。hidden は、そのサービスの番組表に出さない印。
function servicesResponse(hidden: boolean) {
  return {
    data: {
      channelTypes: [],
      services: [
        {
          id: 'U2VydmljZTox',
          name: 'こたつテスト1',
          logoUrl: null,
          hidden,
          sortOrder: null,
          channelType: { displayName: '地上波' },
        },
      ],
      systemSettings: { animeLinkingEnabled: false },
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

    expect(sent).toEqual([{ hidden: true, resetSortOrder: false }]);
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
