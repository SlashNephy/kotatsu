import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { createTestEnvironment, renderRoute } from '../../test/route';
import { Route } from './storage';

afterEach(cleanup);

const gb = 1e9;

// serve は、空き容量が 100 GB、閾値が 10 GB のプールを返し、プールの保存を sent に積む環境を作る。
// 見つからない録画などの欄の問い合わせには、応えずに待たせる。
function serve() {
  const sent: unknown[] = [];
  const environment = createTestEnvironment((request, variables) => {
    if (request.name === 'storagePoolMutation') {
      sent.push(variables.input);

      return {
        data: { updateStoragePool: { id: 'main', priority: 0, minFreeBytes: 10 * gb } },
      };
    }
    if (request.name !== 'storageQuery') {
      return undefined;
    }

    return {
      data: {
        systemStatus: {
          pools: [
            {
              id: 'main',
              path: '/rec',
              priority: 0,
              minFreeBytes: 10 * gb,
              freeBytes: 100 * gb,
              totalBytes: 1000 * gb,
            },
          ],
        },
      },
    };
  });

  return { environment, sent };
}

describe('保存先プールの空き容量の閾値 (#1740)', () => {
  test('今の空き容量までなら、欄を離れたらすぐ保存する', async () => {
    const { environment, sent } = serve();
    await renderRoute(Route, '/admin/storage', '/admin/storage', environment);
    const minFree = await screen.findByRole('textbox', { name: /容量の閾値/ });
    fireEvent.change(minFree, { target: { value: '50' } });

    await act(async () => fireEvent.blur(minFree));
    await screen.findByRole('status');

    expect(sent).toEqual([{ minFreeBytes: 50 * gb }]);
  });

  test('今の空き容量を超えるなら、確かめてから保存し、やめたら保存してある値に戻す', async () => {
    const { environment, sent } = serve();
    await renderRoute(Route, '/admin/storage', '/admin/storage', environment);
    const minFree = await screen.findByRole('textbox', { name: /容量の閾値/ });
    fireEvent.change(minFree, { target: { value: '200' } });

    await act(async () => fireEvent.blur(minFree));

    expect(sent).toEqual([]);
    await act(async () => fireEvent.click(screen.getByRole('button', { name: 'やめる' })));
    expect(screen.getByRole('textbox', { name: /容量の閾値/ })).toHaveProperty('value', '10');

    fireEvent.change(screen.getByRole('textbox', { name: /容量の閾値/ }), {
      target: { value: '200' },
    });
    await act(async () => fireEvent.blur(screen.getByRole('textbox', { name: /容量の閾値/ })));
    await act(async () => fireEvent.click(screen.getByRole('button', { name: '保存する' })));
    await screen.findByRole('status');

    expect(sent).toEqual([{ minFreeBytes: 200 * gb }]);
  });
});
